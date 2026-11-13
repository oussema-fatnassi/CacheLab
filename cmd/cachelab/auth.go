package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// validAPIKey accepts the token syntax used by Authorization: Bearer.
func validAPIKey(key string) bool {
	token := strings.TrimRight(key, "=")
	if token == "" {
		return false
	}
	for _, char := range token {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("-._~+/", char)) {
			return false
		}
	}
	return true
}

// authenticate runs before routing and body processing, including future routes.
func authenticate(next http.Handler, apiKey string) http.Handler {
	expected := sha256.Sum256([]byte(apiKey))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			switch r.URL.Path {
			case "/", "/health", "/swagger", "/swagger/", "/swagger/openapi.yaml":
				next.ServeHTTP(w, r)
				return
			}
		}
		headers := r.Header.Values("Authorization")
		if len(headers) == 1 {
			scheme, token, ok := strings.Cut(headers[0], " ")
			actual := sha256.Sum256([]byte(token))
			if ok && strings.EqualFold(scheme, "Bearer") && token != "" && apiKey != "" && subtle.ConstantTimeCompare(actual[:], expected[:]) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized")
	})
}
