package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// validAPIKey accepts ASCII letters, digits, and -._~+/ followed by optional
// trailing '=' padding. The token must contain at least one non-padding character;
// whitespace, non-ASCII characters, and padding within the token are rejected.
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

// authenticate requires one Authorization header containing a Bearer credential.
// Only GET and HEAD requests to the explicit public paths bypass this check.
// It runs before routing and body processing, protecting future routes by default.
func authenticate(next http.Handler, apiKey string) http.Handler {
	// Compare fixed-size digests so the constant-time comparison does not depend
	// on the length of the supplied credential. The API key itself is not logged.
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
