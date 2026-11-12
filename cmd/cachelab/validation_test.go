package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// spyTable counts the calls it receives so tests can check the store is never touched.
type spyTable struct{ calls int }

func (s *spyTable) Create(key, value string) error { s.calls++; return nil }
func (s *spyTable) Read(key string) (string, bool) { s.calls++; return "", false }
func (s *spyTable) Update(key, value string) error { s.calls++; return nil }
func (s *spyTable) Delete(key string) bool         { s.calls++; return false }

// sendWithLimits passes a request through the real routes with small limits:
// keys up to 4 bytes, values up to 5 bytes and bodies up to 64 bytes.
func sendWithLimits(table cache.Table, method, path, body string) *httptest.ResponseRecorder {
	server := newCacheServer(serverConfig{maxBodyBytes: 64}, cache.NewCacheService(table, 4, 5))
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
	return recorder
}

func TestValidationRejectsInvalidInputWithoutTouchingStore(t *testing.T) {
	tooLarge := strings.Repeat("x", 64)
	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"malformed JSON", http.MethodPost, "/keys", `{"key": "new"`, http.StatusBadRequest},
		{"data after the object", http.MethodPost, "/keys", `{"key": "new", "value": "v"}}`, http.StatusBadRequest},
		{"wrong type", http.MethodPost, "/keys", `{"key": "new", "value": 5}`, http.StatusBadRequest},
		{"null value", http.MethodPost, "/keys", `{"key": "new", "value": null}`, http.StatusBadRequest},
		{"missing value", http.MethodPost, "/keys", `{"key": "new"}`, http.StatusBadRequest},
		{"unknown field", http.MethodPost, "/keys", `{"key": "new", "value": "v", "ttl": 60}`, http.StatusBadRequest},
		{"empty key", http.MethodPost, "/keys", `{"key": "", "value": "v"}`, http.StatusBadRequest},
		{"key one byte over", http.MethodPost, "/keys", `{"key": "abcde", "value": "v"}`, http.StatusBadRequest},
		{"value one byte over", http.MethodPost, "/keys", `{"key": "new", "value": "123456"}`, http.StatusBadRequest},
		{"body too large", http.MethodPost, "/keys", `{"key": "new", "value": "` + tooLarge + `"}`, http.StatusRequestEntityTooLarge},
		{"update malformed JSON", http.MethodPut, "/keys/name", `{"value":`, http.StatusBadRequest},
		{"update empty object", http.MethodPut, "/keys/name", `{}`, http.StatusBadRequest},
		{"update wrong type", http.MethodPut, "/keys/name", `{"value": true}`, http.StatusBadRequest},
		{"update unknown field", http.MethodPut, "/keys/name", `{"value": "v", "ttl": 60}`, http.StatusBadRequest},
		{"update key one byte over", http.MethodPut, "/keys/abcde", `{"value": "v"}`, http.StatusBadRequest},
		{"update value one byte over", http.MethodPut, "/keys/name", `{"value": "123456"}`, http.StatusBadRequest},
		{"update body too large", http.MethodPut, "/keys/name", `{"value": "` + tooLarge + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &spyTable{}
			if recorder := sendWithLimits(spy, tt.method, tt.path, tt.body); recorder.Code != tt.want {
				t.Fatalf("status = %d; want %d", recorder.Code, tt.want)
			}
			if spy.calls != 0 {
				t.Fatalf("store called %d times; want 0", spy.calls)
			}
		})
	}
}

func TestValidationAcceptsInputAtTheLimit(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		// "éé" has 2 characters but 4 bytes: the limits count bytes.
		{"key at the limit", http.MethodPost, "/keys", `{"key": "éé", "value": "v"}`, http.StatusCreated},
		{"value at the limit", http.MethodPost, "/keys", `{"key": "name", "value": "12345"}`, http.StatusCreated},
		{"empty value", http.MethodPost, "/keys", `{"key": "name", "value": ""}`, http.StatusCreated},
		{"update value at the limit", http.MethodPut, "/keys/name", `{"value": "12345"}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &spyTable{}
			if recorder := sendWithLimits(spy, tt.method, tt.path, tt.body); recorder.Code != tt.want {
				t.Fatalf("status = %d; want %d", recorder.Code, tt.want)
			}
			if spy.calls != 1 {
				t.Fatalf("store called %d times; want 1", spy.calls)
			}
		})
	}
}
