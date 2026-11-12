package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// createRequest is the JSON body expected by POST /keys.
type createRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// readResponse is the JSON body returned by GET /keys/{key}.
type readResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// updateRequest is the JSON body expected by PUT /keys/{key}.
type updateRequest struct {
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

// handleRead returns the value stored for the key given in the URL.
func handleRead(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")

		value, err := service.Read(key)
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(readResponse{Key: key, Value: value}); err != nil {
				log.Printf("writing response: %v", err)
			}
		case errors.Is(err, cache.ErrNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, cache.ErrInvalidKey):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}

// handleUpdate replaces the value of an existing key and never creates a new one.
func handleUpdate(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request updateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		err := service.Update(r.PathValue("key"), request.Value)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, cache.ErrNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, cache.ErrInvalidKey), errors.Is(err, cache.ErrValueTooLarge):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}

// handleDelete removes an existing key and answers without a body.
func handleDelete(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := service.Delete(r.PathValue("key"))
		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, cache.ErrNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, cache.ErrInvalidKey):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}
