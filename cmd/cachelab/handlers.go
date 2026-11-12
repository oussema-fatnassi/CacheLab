package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// createRequest is the JSON body expected by POST /keys.
type createRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// handleCreate stores a new key/value pair and refuses an existing key.
func handleCreate(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		err := service.Create(request.Key, request.Value)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusCreated)
		case errors.Is(err, cache.ErrKeyExists):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, cache.ErrInvalidKey), errors.Is(err, cache.ErrValueTooLarge):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			// Any other storage error must never be reported as a success.
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}
