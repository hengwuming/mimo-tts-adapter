package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

type Validator struct {
	token []byte
}

func New(token string) Validator {
	return Validator{token: []byte(token)}
}

func (v Validator) Authorized(headerValues []string) bool {
	if len(headerValues) != 1 {
		return false
	}
	parts := strings.Split(headerValues[0], " ")
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), v.token) == 1
}

func (v Validator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !v.Authorized(r.Header.Values("Authorization")) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"authentication required"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
