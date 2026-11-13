package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

func TestHTTPErrorMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{"invalid key", cache.ErrInvalidKey, http.StatusBadRequest, "invalid key"},
		{"value too large", cache.ErrValueTooLarge, http.StatusBadRequest, "value too large"},
		{"not found", cache.ErrNotFound, http.StatusNotFound, "key not found"},
		{"key exists", cache.ErrKeyExists, http.StatusConflict, "key already exists"},
		// A wrapped error keeps its status, but the wrapping text is never shown.
		{"wrapped error", fmt.Errorf("bucket 3: %w", cache.ErrKeyExists), http.StatusConflict, "key already exists"},
		{"unknown error", errors.New("disk /var/cache is full"), http.StatusInternalServerError, "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, message := httpError(tt.err); status != tt.wantStatus || message != tt.wantMessage {
				t.Fatalf("httpError = (%d, %q); want (%d, %q)", status, message, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}

func TestErrorResponsesUseCommonEnvelope(t *testing.T) {
	// The stored value must never appear in an error response.
	service := cache.NewCacheService(cache.NewHashTable(16), 4, 8)
	if err := service.Create("name", "s3cr3t"); err != nil {
		t.Fatal(err)
	}
	server := newCacheServer(serverConfig{apiKey: testAPIKey, maxBodyBytes: 64}, service).httpServer.Handler
	// failingTable fails with "storage failure", an internal detail to keep hidden.
	failing := newCacheServer(serverConfig{apiKey: testAPIKey, maxBodyBytes: 64}, cache.NewCacheService(failingTable{}, 4, 8)).httpServer.Handler

	tests := []struct {
		name               string
		handler            http.Handler
		method, path, body string
		want               int
	}{
		{"invalid JSON", server, http.MethodPost, "/keys", `{"key":`, http.StatusBadRequest},
		{"invalid key", server, http.MethodGet, "/keys/abcde", "", http.StatusBadRequest},
		{"delete invalid key", server, http.MethodDelete, "/keys/abcde", "", http.StatusBadRequest},
		{"value too large", server, http.MethodPut, "/keys/name", `{"value": "123456789"}`, http.StatusBadRequest},
		{"missing key", server, http.MethodDelete, "/keys/none", "", http.StatusNotFound},
		{"existing key", server, http.MethodPost, "/keys", `{"key": "name", "value": "other"}`, http.StatusConflict},
		{"body too large", server, http.MethodPost, "/keys", `{"key": "new", "value": "` + strings.Repeat("x", 64) + `"}`, http.StatusRequestEntityTooLarge},
		{"storage failure", failing, http.MethodPost, "/keys", `{"key": "new", "value": "v"}`, http.StatusInternalServerError},
		{"update storage failure", failing, http.MethodPut, "/keys/name", `{"value": "v"}`, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			// Valid credentials let each case reach the error layer under test.
			tt.handler.ServeHTTP(recorder, authorizedRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			if recorder.Code != tt.want {
				t.Fatalf("status = %d; want %d", recorder.Code, tt.want)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q; want application/json", got)
			}
			var body errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Error == "" {
				t.Fatalf("body %q is not a JSON error envelope", recorder.Body.String())
			}
			for _, hidden := range []string{"s3cr3t", "storage failure"} {
				if strings.Contains(recorder.Body.String(), hidden) {
					t.Fatalf("body %q leaks %q", recorder.Body.String(), hidden)
				}
			}
		})
	}
}

func TestServerRecoversFromHandlerPanic(t *testing.T) {
	// Without a service, the create handler panics on a nil pointer.
	handler := newCacheServer(serverConfig{apiKey: testAPIKey, maxBodyBytes: 64}, nil).httpServer.Handler

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authorizedRequest(http.MethodPost, "/keys", strings.NewReader(`{"key": "k", "value": "v"}`)))
	var body errorResponse
	if recorder.Code != http.StatusInternalServerError || json.Unmarshal(recorder.Body.Bytes(), &body) != nil || body.Error != "internal server error" {
		t.Fatalf("response = %d %q; want 500 with the generic JSON error", recorder.Code, recorder.Body.String())
	}

	// The server keeps answering the next requests.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status after a panic = %d; want 200", recorder.Code)
	}
}
