package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

func captureRequestLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &output
}

func readLogEvents(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("invalid JSON log: %v", err)
		}
		events = append(events, event)
	}
	return events
}

func TestRequestLogging(t *testing.T) {
	output := captureRequestLogs(t)
	service := cache.NewCacheService(cache.NewHashTable(4), 256, 1024)
	handler := newCacheServer(serverConfig{maxBodyBytes: 128}, service).httpServer.Handler
	seenIDs := make(map[string]bool)

	for _, test := range []struct {
		name, method, path, body, route string
		status                          int
	}{
		{"create", "POST", "/keys", `{"key":"private-key","value":"private-value"}`, "POST /keys", 201},
		{"read", "GET", "/keys/private-key?token=private-query", "", "GET /keys/{key}", 200},
		{"duplicate", "POST", "/keys", `{"key":"private-key","value":"private-value"}`, "POST /keys", 409},
		{"update", "PUT", "/keys/private-key", `{"value":"updated-private-value"}`, "PUT /keys/{key}", 200},
		{"delete", "DELETE", "/keys/private-key", "", "DELETE /keys/{key}", 204},
		{"missing", "GET", "/keys/private-key", "", "GET /keys/{key}", 404},
		{"invalid JSON", "POST", "/keys", "invalid-private-body\nforged-event", "POST /keys", 400},
		{"body limit", "POST", "/keys", strings.Repeat("x", 129), "POST /keys", 413},
		{"implicit status", "GET", "/", "", "/", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			output.Reset()
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			req.Header.Set("Authorization", "Bearer private-credential")
			req.Header.Set("X-Request-ID", "private-client-id\nforged-event")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)

			events := readLogEvents(t, output)
			if len(events) != 1 {
				t.Fatalf("got %d log events; want 1", len(events))
			}
			event := events[0]
			if response.Code != test.status || event["status"] != float64(test.status) {
				t.Fatalf("response/log status = %d/%v; want %d", response.Code, event["status"], test.status)
			}
			if event["method"] != test.method || event["route"] != test.route || event["msg"] != "http_request" {
				t.Fatalf("unexpected request fields: %v", event)
			}
			id, ok := event["request_id"].(string)
			if !ok || id == "" || id != response.Header().Get("X-Request-ID") || seenIDs[id] {
				t.Fatal("missing, duplicated or mismatched request ID")
			}
			seenIDs[id] = true
			if duration, ok := event["duration_ms"].(float64); !ok || duration < 0 {
				t.Fatal("missing or invalid duration_ms")
			}
			if event["level"] != "INFO" || event["time"] == nil || len(event) != 8 {
				t.Fatalf("unexpected log field set: %v", event)
			}
			for _, hidden := range []string{"private-", "forged-event", "Authorization", "?token="} {
				if strings.Contains(output.String(), hidden) {
					t.Fatalf("log exposes request data matching %q", hidden)
				}
			}
		})
	}
}

func TestRequestLoggingHidesInternalDetails(t *testing.T) {
	output := captureRequestLogs(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /panic/{key}", func(w http.ResponseWriter, r *http.Request) {
		panic("private-value\nforged-event")
	})
	mux.HandleFunc("GET /error/{key}", func(w http.ResponseWriter, r *http.Request) {
		writeServiceError(w, errors.New("private-value\nforged-event"))
	})
	handler := logRequests(slog.Default(), recoverPanics(mux))

	for _, path := range []string{"/panic/private-key", "/error/private-key"} {
		output.Reset()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d; want 500", response.Code)
		}
		if strings.Contains(output.String(), "private-") || strings.Contains(output.String(), "forged-event") {
			t.Fatal("internal error logs expose sensitive data")
		}
		events := readLogEvents(t, output)
		if len(events) != 2 || events[0]["level"] != "ERROR" || events[1]["status"] != float64(500) {
			t.Fatalf("expected a generic error event and request status 500: %v", events)
		}
		for _, event := range events {
			if event["request_id"] != response.Header().Get("X-Request-ID") {
				t.Fatal("error and request events must share their request ID")
			}
		}
	}
}

func TestRequestLoggingConcurrent(t *testing.T) {
	output := captureRequestLogs(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /keys/{key}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := logRequests(slog.Default(), mux)
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/keys/private-key", nil))
		}()
	}
	workers.Wait()
	ids := make(map[string]bool)
	events := readLogEvents(t, output)
	if len(events) != 20 {
		t.Fatalf("got %d events; want 20", len(events))
	}
	for _, event := range events {
		id, ok := event["request_id"].(string)
		if !ok || id == "" || ids[id] || event["status"] != float64(204) {
			t.Fatal("invalid concurrent request event")
		}
		ids[id] = true
	}
}

func TestResponseStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		write  func(http.ResponseWriter)
		status int
	}{
		{"empty response", func(w http.ResponseWriter) {}, 200},
		{"first final status wins", func(w http.ResponseWriter) {
			w.WriteHeader(201)
			w.WriteHeader(409)
		}, 201},
		{"informational then final", func(w http.ResponseWriter) {
			w.WriteHeader(103)
			w.WriteHeader(204)
		}, 204},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			handler := logRequests(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				test.write(w)
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
			if event := readLogEvents(t, &output)[0]; event["status"] != float64(test.status) {
				t.Fatalf("logged status = %v; want %d", event["status"], test.status)
			}
		})
	}
}
