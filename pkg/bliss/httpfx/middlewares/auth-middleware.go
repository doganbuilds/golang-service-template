package middlewares

import (
	"errors"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/eser/go-service/pkg/bliss/httpfx"
)

var ErrInvalidSigningMethod = errors.New("Invalid signing method")

func AuthMiddleware() httpfx.Handler {
	return func(ctx *httpfx.Context) httpfx.Response {
		authHeader := ctx.Request.Header.Get("Authorization")

		if authHeader == "" {
			return ctx.Results.Unauthorized("Authorization header is missing")
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == "" {
			return ctx.Results.Unauthorized("Token is missing")
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidSigningMethod
			}

			return []byte("secret"), nil
		})

		if err != nil || !token.Valid {
			return ctx.Results.Unauthorized(err.Error())
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			return ctx.Results.Unauthorized("Invalid token")
		}

		if exp, ok := claims["exp"].(float64); ok {
			if time.Unix(int64(exp), 0).Before(time.Now()) {
				return ctx.Results.Unauthorized("Token is expired")
			}
		}

		return ctx.Next()
	}
}
