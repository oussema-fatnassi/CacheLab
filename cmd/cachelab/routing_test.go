package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

func TestServerRoutingErrorsAreJSON(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, allow string
		status                    int
	}{
		{"GET", "/unknown", `{"error":"route not found"}`, "", 404},
		{"GET", "/swagger/unknown", `{"error":"route not found"}`, "", 404},
		{"GET", "/keys/missing", `{"error":"key not found"}`, "", 404},
		{"GET", "/keys", `{"error":"method not allowed"}`, "POST", 405},
		{"POST", "/keys/name", `{"error":"method not allowed"}`, "DELETE, GET, HEAD, PUT", 405},
		{"POST", "/health", `{"error":"method not allowed"}`, "GET, HEAD", 405},
		{"POST", "/metrics", `{"error":"method not allowed"}`, "GET, HEAD", 405},
		{"POST", "/swagger/", `{"error":"method not allowed"}`, "GET, HEAD", 405},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var logs bytes.Buffer
			handler := loggedHandler(cache.NewCacheService(cache.NewHashTable(16), 64, 64), &logs)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authorizedRequest(tc.method, tc.path, nil))
			if response.Code != tc.status || response.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(response.Body.String()) != tc.body {
				t.Fatalf("response = %d %q %q; want JSON %d %s", response.Code, response.Header().Get("Content-Type"), response.Body.String(), tc.status, tc.body)
			}
			if got := response.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow = %q; want %q", got, tc.allow)
			}
			event := requestEvent(t, &logs)
			requestID := response.Header().Get("X-Request-ID")
			if requestID == "" || event["request_id"] != requestID || event["status"] != float64(tc.status) {
				t.Fatalf("request ID/status not preserved: header %q, event %v", requestID, event)
			}
			if tc.path == "/keys/missing" {
				assertMetricLines(t, scrapeMetrics(t, handler), "cachelab_cache_misses_total 1")
			} else {
				assertMetricLines(t, scrapeMetrics(t, handler), "cachelab_cache_misses_total 0")
			}
		})
	}
}

func TestServerAuthenticatesBeforeRoutingErrors(t *testing.T) {
	handler := metricsHandler()
	for _, tc := range []struct{ method, path string }{
		{"GET", "/unknown"}, {"POST", "/health"}, {"GET", "/keys"}, {"POST", "/keys/name"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			assertUnauthorized(t, response)
			if got := response.Header().Get("Allow"); got != "" {
				t.Fatalf("unauthenticated request disclosed Allow = %q", got)
			}
		})
	}
}

func TestServerRoutingRejectsAsteriskTarget(t *testing.T) {
	response := httptest.NewRecorder()
	metricsHandler().ServeHTTP(response, authorizedRequest(http.MethodGet, "*", nil))
	if response.Code != http.StatusBadRequest || response.Header().Get("Connection") != "close" || response.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(response.Body.String()) != `{"error":"bad request"}` {
		t.Fatalf("asterisk target response = %d %v %q; want JSON 400 and Connection: close", response.Code, response.Header(), response.Body.String())
	}
}

func TestServerOPTIONSStarIsHandledBeforeApplication(t *testing.T) {
	server := httptest.NewServer(metricsHandler())
	defer server.Close()
	request, err := http.NewRequest(http.MethodOptions, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.URL.Opaque = "*"
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(body) != 0 || response.Header.Get("X-Request-ID") != "" {
		t.Fatalf("OPTIONS * response = %d %v %q; want net/http's empty 200 before the application", response.StatusCode, response.Header, body)
	}
}

func TestServerRoutingPreservesRedirects(t *testing.T) {
	mux := registerRoutes(routeHandlers{}, 64)
	handler := metricsHandler()
	for _, path := range []string{"/swagger?view=api", "/swagger//openapi.yaml?view=api", "/unknown//path?view=api", "/unknown/../a%2Fb?view=api"} {
		t.Run(path, func(t *testing.T) {
			want := httptest.NewRecorder()
			mux.ServeHTTP(want, authorizedRequest(http.MethodGet, path, nil))
			got := httptest.NewRecorder()
			handler.ServeHTTP(got, authorizedRequest(http.MethodGet, path, nil))
			if want.Code < 300 || want.Code >= 400 {
				t.Fatalf("baseline route did not redirect: %d", want.Code)
			}
			if got.Code != want.Code || got.Header().Get("Location") != want.Header().Get("Location") || got.Body.String() != want.Body.String() {
				t.Fatalf("redirect = %d %q %q; want %d %q %q", got.Code, got.Header().Get("Location"), got.Body.String(), want.Code, want.Header().Get("Location"), want.Body.String())
			}
		})
	}
}

func TestServerRoutingPreservesPathValuesAndPublicHandlers(t *testing.T) {
	table := cache.NewHashTable(16)
	if err := table.Create("a/b", "stored"); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	handler := loggedHandler(cache.NewCacheService(table, 64, 64), &logs)
	for _, tc := range []struct{ path, content string }{
		{"/keys/a%2Fb", `{"key":"a/b","value":"stored"}`},
		{"/", "Hello, World!"},
		{"/health", `{"status":"ok"}`},
		{"/swagger/", "SwaggerUIBundle"},
		{"/swagger/openapi.yaml", "openapi: 3.0.3"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if strings.HasPrefix(tc.path, "/keys/") {
				request.Header.Set("Authorization", "Bearer "+testAPIKey)
			}
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), tc.content) {
				t.Fatalf("response = %d %q; want 200 containing %q", response.Code, response.Body.String(), tc.content)
			}
		})
	}
}

func TestServerHEADResponsesHaveNoWireBody(t *testing.T) {
	server := httptest.NewServer(metricsHandler())
	defer server.Close()
	client := server.Client()
	for _, tc := range []struct {
		path       string
		authorized bool
		status     int
		allow      string
	}{
		{"/unknown", true, 404, ""},
		{"/keys", true, 405, "POST"},
		{"/keys/missing", true, 404, ""},
		{"/unknown", false, 401, ""},
		{"/health", false, 200, ""},
	} {
		t.Run(tc.path+"/"+http.StatusText(tc.status), func(t *testing.T) {
			request, err := http.NewRequest(http.MethodHead, server.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.authorized {
				request.Header.Set("Authorization", "Bearer "+testAPIKey)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := response.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status || response.Header.Get("Content-Type") != "application/json" || len(body) != 0 || response.Header.Get("Allow") != tc.allow {
				t.Fatalf("HEAD response = %d %q %q Allow=%q; want JSON %d, empty body, Allow=%q", response.StatusCode, response.Header.Get("Content-Type"), body, response.Header.Get("Allow"), tc.status, tc.allow)
			}
		})
	}
}
