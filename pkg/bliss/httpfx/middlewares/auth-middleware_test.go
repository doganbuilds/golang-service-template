package middlewares_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/eser/go-service/pkg/bliss/httpfx"
	"github.com/eser/go-service/pkg/bliss/httpfx/middlewares"
)

func createToken(secret string, exp time.Time) string {
	claims := jwt.MapClaims{
		"exp": exp.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))

	return tokenString
}

func TestAuthMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		token              string
		expectedStatusCode int
	}{
		{
			name:               "No Authorization Header",
			token:              "",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "Invalid Token Format",
			token:              "InvalidToken",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "Expired Token",
			token:              createToken("secret", time.Now().Add(-time.Hour)),
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "Valid Token with Invalid Secret",
			token:              createToken("secret2", time.Now().Add(time.Hour)),
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "Valid Token",
			token:              createToken("secret", time.Now().Add(time.Hour)),
			expectedStatusCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)

			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			res := httptest.NewRecorder()
			httpCtx := httpfx.Context{ //nolint:exhaustruct
				Request:        req,
				ResponseWriter: res,
			}

			middleware := middlewares.AuthMiddleware()
			result := middleware(&httpCtx)

			if result.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, result.StatusCode)
			}

			if tt.expectedStatusCode == http.StatusOK || tt.expectedStatusCode == http.StatusNoContent {
				claims, claimsOk := httpCtx.Request.Context().Value(middlewares.KeyClaims).(jwt.MapClaims)

				if !claimsOk {
					t.Error("Claims are missing in context")
				}

				if claims["exp"] == nil {
					t.Error("exp claim is missing")
				}

				if exp, ok := claims["exp"].(float64); ok {
					if time.Unix(int64(exp), 0).Before(time.Now()) {
						t.Error("exp claim is not valid")
					}
				}
			}
		})
	}
}