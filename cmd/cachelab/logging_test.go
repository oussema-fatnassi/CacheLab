package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// loggedHandler returns the real server handler with its logs written to logs.
func loggedHandler(service *cache.CacheService, logs *bytes.Buffer) http.Handler {
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return serverHandler(serverConfig{apiKey: testAPIKey, maxBodyBytes: 64}, service, logger)
}

// logEvents decodes every log line and fails if one is not a JSON object.
func logEvents(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("log line %q is not a JSON object: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

// requestEvent returns the single "http request" event found in logs.
func requestEvent(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, event := range logEvents(t, logs) {
		if event["msg"] == "http request" {
			found = append(found, event)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d request events; want 1:\n%s", len(found), logs.String())
	}
	return found[0]
}

func TestRequestLogsContainExpectedFields(t *testing.T) {
	service := func() *cache.CacheService { return cache.NewCacheService(cache.NewHashTable(16), 20, 20) }
	tests := []struct {
		name    string
		service *cache.CacheService
		request *http.Request
		route   string
		status  int
	}{
		{"created", service(), authorizedRequest(http.MethodPost, "/keys", strings.NewReader(`{"key": "k", "value": "v"}`)), "POST /keys", http.StatusCreated},
		{"missing key", service(), authorizedRequest(http.MethodGet, "/keys/none", nil), "GET /keys/{key}", http.StatusNotFound},
		{"no credentials", service(), httptest.NewRequest(http.MethodPut, "/keys/k", strings.NewReader(`{"value": "v"}`)), "PUT /keys/{key}", http.StatusUnauthorized},
		{"body too large", service(), authorizedRequest(http.MethodPost, "/keys", strings.NewReader(strings.Repeat("x", 65))), "POST /keys", http.StatusRequestEntityTooLarge},
		// Without a service, the delete handler panics and the panic is recovered.
		{"recovered panic", nil, authorizedRequest(http.MethodDelete, "/keys/k", nil), "DELETE /keys/{key}", http.StatusInternalServerError},
		{"public health", service(), httptest.NewRequest(http.MethodGet, "/health", nil), "GET /health", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			recorder := httptest.NewRecorder()
			loggedHandler(tt.service, &logs).ServeHTTP(recorder, tt.request)
			if recorder.Code != tt.status {
				t.Fatalf("status = %d; want %d", recorder.Code, tt.status)
			}

			requestID := recorder.Header().Get("X-Request-ID")
			if requestID == "" {
				t.Fatal("missing X-Request-ID header")
			}
			event := requestEvent(t, &logs)
			// The logged status is the one the client actually received.
			want := map[string]any{"method": tt.request.Method, "route": tt.route, "status": float64(recorder.Code), "request_id": requestID}
			for field, value := range want {
				if event[field] != value {
					t.Fatalf("%s = %v; want %v", field, event[field], value)
				}
			}
			if duration, ok := event["duration_ms"].(float64); !ok || duration < 0 {
				t.Fatalf("duration_ms = %v; want a positive number", event["duration_ms"])
			}
		})
	}
}

func TestRequestLogsHideSensitiveData(t *testing.T) {
	var logs bytes.Buffer
	handler := loggedHandler(cache.NewCacheService(cache.NewHashTable(16), 64, 64), &logs)

	wrongKey := httptest.NewRequest(http.MethodDelete, "/keys/user:42:email", nil)
	wrongKey.Header.Set("Authorization", "Bearer wrong-secret-key")
	requests := []*http.Request{
		authorizedRequest(http.MethodPost, "/keys", strings.NewReader(`{"key": "user:42:email", "value": "alice@example.com"}`)),
		authorizedRequest(http.MethodGet, "/keys/user:42:email", nil),
		authorizedRequest(http.MethodPut, "/keys/user:42:email", strings.NewReader(`{"value": "alice@example.com"}`)),
		httptest.NewRequest(http.MethodGet, "/keys/user:42:email?token=secret-query", nil),
		wrongKey,
	}
	for _, request := range requests {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
	// Without a service, the read handler panics on a nil pointer.
	loggedHandler(nil, &logs).ServeHTTP(httptest.NewRecorder(), authorizedRequest(http.MethodGet, "/keys/user:42:email", nil))

	for _, secret := range []string{"user:42", "alice@example.com", testAPIKey, "wrong-secret-key", "secret-query", "Bearer", "nil pointer"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logs leak %q:\n%s", secret, logs.String())
		}
	}
}

func TestRequestLogsCannotBeInjected(t *testing.T) {
	var logs bytes.Buffer
	handler := loggedHandler(cache.NewCacheService(cache.NewHashTable(16), 64, 64), &logs)

	// %0A is a line break, followed by a forged JSON event inside the key.
	handler.ServeHTTP(httptest.NewRecorder(), authorizedRequest(http.MethodGet, "/keys/a%0A%7B%22msg%22%3A%22forged%22%7D", nil))

	events := logEvents(t, &logs)
	if len(events) != 1 || events[0]["msg"] != "http request" || strings.Contains(logs.String(), "forged") {
		t.Fatalf("got %d events; want only the request event:\n%s", len(events), logs.String())
	}
}
