package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// errorResponse is the JSON body shared by every error response.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError answers with status and the common JSON error body.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorResponse{Error: message}); err != nil {
		log.Printf("writing error response: %v", err)
	}
}

// jsonRoutingErrors replaces only the mux's generated 400, 404 and 405 responses.
// Matched routes still use ServeHTTP to populate wildcard path values. An empty
// pattern can also mean a path-cleaning redirect, which the writer passes through.
func jsonRoutingErrors(routes *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := routes.Handler(r)
		if pattern != "" {
			routes.ServeHTTP(w, r)
			return
		}
		routes.ServeHTTP(&routingErrorWriter{ResponseWriter: w}, r)
	})
}

type routingErrorWriter struct {
	http.ResponseWriter
	replaced bool
}

func (w *routingErrorWriter) WriteHeader(status int) {
	switch status {
	case http.StatusBadRequest:
		w.replaced = true
		writeError(w.ResponseWriter, status, "bad request")
	case http.StatusNotFound:
		w.replaced = true
		writeError(w.ResponseWriter, status, "route not found")
	case http.StatusMethodNotAllowed:
		w.replaced = true
		writeError(w.ResponseWriter, status, "method not allowed")
	default:
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *routingErrorWriter) Write(body []byte) (int, error) {
	if w.replaced {
		return len(body), nil
	}
	return w.ResponseWriter.Write(body)
}

// httpError maps a cache service error to its HTTP status and public message.
// The message comes from the known error itself, never from a wrapped error,
// so no internal detail reaches the client.
func httpError(err error) (int, string) {
	switch {
	case errors.Is(err, cache.ErrInvalidKey):
		return http.StatusBadRequest, cache.ErrInvalidKey.Error()
	case errors.Is(err, cache.ErrValueTooLarge):
		return http.StatusBadRequest, cache.ErrValueTooLarge.Error()
	case errors.Is(err, cache.ErrNotFound):
		return http.StatusNotFound, cache.ErrNotFound.Error()
	case errors.Is(err, cache.ErrKeyExists):
		return http.StatusConflict, cache.ErrKeyExists.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

// writeServiceError answers with the status and message matching err.
func writeServiceError(w http.ResponseWriter, err error) {
	status, message := httpError(err)
	if status == http.StatusInternalServerError {
		// The real cause is kept in the server logs only.
		log.Printf("unexpected service error: %v", err)
	}
	writeError(w, status, message)
}
