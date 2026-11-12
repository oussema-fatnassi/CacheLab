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
