package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// createRequest is the JSON body expected by POST /keys.
// Pointers tell a missing or null field apart from an empty string.
type createRequest struct {
	Key   *string `json:"key"`
	Value *string `json:"value"`
}

// readResponse is the JSON body returned by GET /keys/{key}.
type readResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// updateRequest is the JSON body expected by PUT /keys/{key}.
type updateRequest struct {
	Value *string `json:"value"`
}

// decodeJSON reads exactly one JSON object that contains only the expected fields.
func decodeJSON(r *http.Request, request any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		return err
	}
	// Anything left after the object, such as a second object or a stray brace, is refused.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON object")
	}
	return nil
}

// handleCreate stores a new key/value pair and refuses an existing key.
func handleCreate(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createRequest
		if err := decodeJSON(r, &request); err != nil || request.Key == nil || request.Value == nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := service.Create(*request.Key, *request.Value); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// handleRead returns the value stored for the key given in the URL.
func handleRead(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")

		value, err := service.Read(key)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(readResponse{Key: key, Value: value}); err != nil {
			slog.Error("response write failed", "request_id", w.Header().Get("X-Request-ID"))
		}
	}
}

// handleUpdate replaces the value of an existing key and never creates a new one.
func handleUpdate(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request updateRequest
		if err := decodeJSON(r, &request); err != nil || request.Value == nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := service.Update(r.PathValue("key"), *request.Value); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// handleDelete removes an existing key and answers without a body.
func handleDelete(service *cache.CacheService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := service.Delete(r.PathValue("key")); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
