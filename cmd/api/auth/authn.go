// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kubearchive/kubearchive/pkg/abort"
	"github.com/kubearchive/kubearchive/pkg/cache"

	"github.com/gin-gonic/gin"
	apiAuthnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientAuthnv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
)

func extractBearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("empty authorization bearer token given")
	}

	jwtToken := strings.Split(header, " ")
	if len(jwtToken) != 2 {
		return "", fmt.Errorf("incorrectly formatted authorization header, "+
			"expected two strings separated by a space but found %d", len(jwtToken))
	}

	return jwtToken[1], nil
}

func Authentication(tri clientAuthnv1.TokenReviewInterface, cache *cache.Cache, cacheExpirationAuthorized, cacheExpirationUnauthorized time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			abort.Abort(c, err, http.StatusBadRequest)
			return
		}

		userInfo := cache.Get(token)
		if userInfo != nil {
			if userInfo == false { // Unauthenticated
				abort.Abort(c, errors.New("authentication failed"), http.StatusUnauthorized)
				return
			}

			c.Set("user", userInfo)
			c.Next()
			return
		}

		tr, err := tri.Create(c.Request.Context(), &apiAuthnv1.TokenReview{
			Spec: apiAuthnv1.TokenReviewSpec{
				Token: token,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			abort.Abort(c, fmt.Errorf("unexpected error on TokenReview: %w", err), http.StatusInternalServerError)
			return
		}
		if !tr.Status.Authenticated {
			abort.Abort(c, errors.New("authentication failed"), http.StatusUnauthorized)
			cache.Set(token, false, cacheExpirationUnauthorized)
			return
		}

		cache.Set(token, tr.Status.User, cacheExpirationAuthorized)

		c.Set("user", tr.Status.User)
	}
}
