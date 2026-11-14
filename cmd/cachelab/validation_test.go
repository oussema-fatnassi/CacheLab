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

// sendWithLimits sends an authenticated request through the real middleware and
// routes, so validation failures cannot be masked by a 401. It uses small limits:
// keys up to 4 bytes, values up to 5 bytes and bodies up to 64 bytes.
func sendWithLimits(table cache.Table, method, path, body string) *httptest.ResponseRecorder {
	server := newCacheServer(serverConfig{apiKey: testAPIKey, maxBodyBytes: 64}, cache.NewCacheService(table, 4, 5))
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, authorizedRequest(method, path, strings.NewReader(body)))
	return recorder
}

type validationCase struct {
	name, method, path, body string
	want                     int
}

func invalidInputCases() []validationCase {
	tooLarge := strings.Repeat("x", 64)
	return []validationCase{
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
}

func TestValidationRejectsInvalidInputWithoutTouchingStore(t *testing.T) {
	for _, tt := range invalidInputCases() {
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

func inputAtLimitCases() []validationCase {
	return []validationCase{
		// "éé" has 2 characters but 4 bytes: the limits count bytes.
		{"key at the limit", http.MethodPost, "/keys", `{"key": "éé", "value": "v"}`, http.StatusCreated},
		{"value at the limit", http.MethodPost, "/keys", `{"key": "name", "value": "12345"}`, http.StatusCreated},
		{"empty value", http.MethodPost, "/keys", `{"key": "name", "value": ""}`, http.StatusCreated},
		{"update value at the limit", http.MethodPut, "/keys/name", `{"value": "12345"}`, http.StatusOK},
	}
}

func TestValidationAcceptsInputAtTheLimit(t *testing.T) {
	for _, tt := range inputAtLimitCases() {
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

// Reuse the spy tests' cases against real configured storage and HTTP transport.
func runHTTPValidation(t *testing.T, client *http.Client, baseURL, apiKey string) {
	t.Helper()
	token := "Bearer " + apiKey
	for _, test := range invalidInputCases() {
		response, body := sendHTTP(t, client, baseURL, test.method, test.path, test.body, token)
		if response.StatusCode != test.want || response.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("%s = %d %q; want JSON %d", test.name, response.StatusCode, body, test.want)
		}
	}
	for _, test := range inputAtLimitCases() {
		// Each success case starts with fresh keys, as in the original spy fixture.
		for _, path := range []string{"/keys/name", "/keys/éé"} {
			sendHTTP(t, client, baseURL, "DELETE", path, "", token)
		}
		if test.method == http.MethodPut {
			response, body := sendHTTP(t, client, baseURL, "POST", "/keys", `{"key":"name","value":"old"}`, token)
			if response.StatusCode != http.StatusCreated {
				t.Fatalf("seed update fixture = %d %q", response.StatusCode, body)
			}
		}
		response, body := sendHTTP(t, client, baseURL, test.method, test.path, test.body, token)
		if response.StatusCode != test.want {
			t.Fatalf("%s = %d %q; want %d", test.name, response.StatusCode, body, test.want)
		}
	}
	for _, path := range []string{"/keys/name", "/keys/éé"} {
		sendHTTP(t, client, baseURL, "DELETE", path, "", token)
	}
}
