package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

const testAPIKey = "test-only-key"

func authorizedRequest(method, path string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Authorization", "Bearer "+testAPIKey)
	return request
}

func authenticatedTestServer(t *testing.T, table cache.Table) *CacheServer {
	t.Helper()
	t.Setenv("API_KEY", testAPIKey)
	config, err := loadServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	return newCacheServer(config, cache.NewCacheService(table, 256, 1024))
}

func TestServerConfigurationRequiresValidAPIKey(t *testing.T) {
	for _, value := range []string{"", " ", "\t\n", " key", "key ", "two words", "key\n", "key\x01", "clé", "key=middle", "=only-padding"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("API_KEY", value)
			_, err := loadServerConfig()
			if err == nil {
				t.Fatal("expected invalid API_KEY to prevent startup")
			}
			if strings.TrimSpace(value) != "" && strings.Contains(err.Error(), value) {
				t.Fatal("configuration error exposed API_KEY")
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		t.Setenv("API_KEY", "")
		// Unset distinguishes an absent variable from an explicitly empty value.
		if err := os.Unsetenv("API_KEY"); err != nil {
			t.Fatal(err)
		}
		if _, err := loadServerConfig(); err == nil {
			t.Fatal("expected missing API_KEY to prevent startup")
		}
	})
}

func TestServerConfigurationAcceptsBearerTokenCharacters(t *testing.T) {
	t.Setenv("API_KEY", "test-._~+/012ABC==")
	if _, err := loadServerConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestUnauthorizedCRUDNeverTouchesStorage(t *testing.T) {
	for _, route := range []struct{ method, path, body string }{
		{"POST", "/keys", `{"key":"name","value":"Test"}`},
		{"GET", "/keys/name", ""},
		{"PUT", "/keys/name", `{"value":"Alice"}`},
		{"DELETE", "/keys/name", ""},
	} {
		// Reject ambiguous credentials as well as incorrect ones: extra spaces,
		// comma-joined tokens, and duplicate headers must not authenticate.
		for _, headers := range [][]string{
			nil, {""}, {"Bearer"}, {"Bearer "}, {"Basic " + testAPIKey},
			{"Bearer wrong-key"}, {"Bearer test-only-kez"}, {"Bearer  " + testAPIKey},
			{"Bearer " + testAPIKey + " "}, {"Bearer " + testAPIKey + ",other"},
			{"Bearer " + testAPIKey, "Bearer " + testAPIKey},
		} {
			t.Run(route.method+"/"+strings.Join(headers, ","), func(t *testing.T) {
				table := &spyTable{}
				server := authenticatedTestServer(t, table)
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				for _, header := range headers {
					request.Header.Add("Authorization", header)
				}
				response := httptest.NewRecorder()
				server.httpServer.Handler.ServeHTTP(response, request)
				assertUnauthorized(t, response)
				if table.calls != 0 {
					t.Fatalf("unauthorized request reached storage %d times", table.calls)
				}
			})
		}
	}
}

// unreadBody makes any attempt to consume an unauthenticated body observable.
type unreadBody struct{ reads int }

func (b *unreadBody) Read([]byte) (int, error) { b.reads++; return 0, http.ErrBodyReadAfterClose }
func (b *unreadBody) Close() error             { return nil }

func TestUnauthorizedRequestDoesNotReadBody(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			server := authenticatedTestServer(t, &spyTable{})
			path := "/keys"
			if method == http.MethodPut {
				path += "/name"
			}
			request := httptest.NewRequest(method, path, nil)
			body := &unreadBody{}
			request.Body = body
			request.ContentLength = 1 << 30
			response := httptest.NewRecorder()
			server.httpServer.Handler.ServeHTTP(response, request)
			assertUnauthorized(t, response)
			if body.reads != 0 {
				t.Fatalf("unauthorized body read %d times", body.reads)
			}
		})
	}
}

func assertUnauthorized(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusUnauthorized || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response = %d %q %q; want JSON 401", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error != "unauthorized" {
		t.Fatalf("unexpected error response: %q (%v)", response.Body.String(), err)
	}
	if response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatal("missing Bearer authentication challenge")
	}
}

func TestAuthorizedCRUD(t *testing.T) {
	for _, engine := range []string{"chained", "recursive", "go-map"} {
		t.Run(engine, func(t *testing.T) {
			for _, setting := range serverTestEnvironment(engine, "8080") {
				name, value, _ := strings.Cut(setting, "=")
				t.Setenv(name, value)
			}
			app, err := newApplication()
			if err != nil {
				t.Fatal(err)
			}
			config, err := loadServerConfig()
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(newCacheServer(config, app.service).httpServer.Handler)
			t.Cleanup(server.Close)
			client := server.Client()
			client.Timeout = 2 * time.Second
			runConfiguredHTTPConformance(t, client, server.URL, testAPIKey)
		})
	}
	t.Run("recursive-configured", func(t *testing.T) {
		table, err := cache.NewRecursiveTableWithConfig(cache.RecursiveConfig{
			BranchCount: 4, MaxDepth: 2, Hash: func(string) uint64 { return 7 },
		})
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(authenticatedTestServer(t, table).httpServer.Handler)
		t.Cleanup(server.Close)
		client := server.Client()
		client.Timeout = 2 * time.Second
		runAuthorizedCRUD(t, client, server.URL, testAPIKey)
	})
}

// The same small, explicit limits are used in-process and in the real binary.
func serverTestEnvironment(engine, port string) []string {
	return []string{
		"CACHE_ENGINE=" + engine, "CACHE_BUCKET_COUNT=2", "API_KEY=" + testAPIKey,
		"MAX_KEY_BYTES=4", "MAX_VALUE_BYTES=5", "MAX_BODY_BYTES=64",
		"PORT=" + port, "SHUTDOWN_TIMEOUT=1s",
	}
}

// sendHTTP consumes and closes each response; callers can inspect status and headers.
func sendHTTP(t *testing.T, client *http.Client, baseURL, method, path, body, token string) (*http.Response, string) {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("reading response: %v; closing response: %v", err, closeErr)
	}
	return response, strings.TrimSpace(string(data))
}

func runAuthorizedCRUD(t *testing.T, client *http.Client, baseURL, apiKey string) {
	t.Helper()
	for _, step := range []struct {
		method, path, body, wantBody string
		want                         int
	}{
		{"POST", "/keys", `{"key":"name","value":"Test"}`, "", 201},
		{"POST", "/keys", `{"key":"name","value":"other"}`, `{"error":"key already exists"}`, 409},
		{"GET", "/keys/name", "", `{"key":"name","value":"Test"}`, 200},
		{"PUT", "/keys/name", `{"value":"Alice"}`, "", 200},
		{"GET", "/keys/name", "", `{"key":"name","value":"Alice"}`, 200},
		{"DELETE", "/keys/name", "", "", 204},
		{"GET", "/keys/name", "", `{"error":"key not found"}`, 404},
	} {
		response, body := sendHTTP(t, client, baseURL, step.method, step.path, step.body, "bEaReR "+apiKey)
		if response.StatusCode != step.want || body != step.wantBody {
			t.Fatalf("%s %s = %d %q; want %d %q", step.method, step.path, response.StatusCode, body, step.want, step.wantBody)
		}
		if response.Header.Get("X-Request-ID") == "" {
			t.Fatal("missing request logging middleware")
		}
	}
}

func runConfiguredHTTPConformance(t *testing.T, client *http.Client, baseURL, apiKey string) {
	t.Helper()
	runAuthorizedCRUD(t, client, baseURL, apiKey)
	runHTTPValidation(t, client, baseURL, apiKey)
	for _, step := range []struct{ method, path, body, token string }{
		{"GET", "/keys/name", "", ""},
		{"GET", "/metrics", "", ""},
		{"GET", "/metrics", "", "Bearer wrong-key"},
		{"POST", "/keys", strings.Repeat("x", 65), ""},
	} {
		response, body := sendHTTP(t, client, baseURL, step.method, step.path, step.body, step.token)
		if response.StatusCode != 401 || body != `{"error":"unauthorized"}` || response.Header.Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("unauthenticated %s %s = %d %q", step.method, step.path, response.StatusCode, body)
		}
	}
	response, body := sendHTTP(t, client, baseURL, "GET", "/health", "", "")
	if response.StatusCode != 200 || body != `{"status":"ok"}` {
		t.Fatalf("health = %d %q", response.StatusCode, body)
	}
	response, body = sendHTTP(t, client, baseURL, "GET", "/metrics", "", "Bearer "+apiKey)
	if response.StatusCode != 200 || !strings.Contains(body, "cachelab_cache_hits_total") {
		t.Fatalf("metrics = %d %q", response.StatusCode, body)
	}
}

func TestHealthIsPublicAndContainsNoCacheData(t *testing.T) {
	table := cache.NewHashTable(16)
	if err := table.Create("private-key", "private-value"); err != nil {
		t.Fatal(err)
	}
	server := authenticatedTestServer(t, table)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/health", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(response.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
}

func TestMetricsIsProtected(t *testing.T) {
	// The metrics exporter answers only with a valid key, like the CRUD routes.
	server := authenticatedTestServer(t, &spyTable{})
	for _, token := range []string{"", "Bearer wrong-key", "Bearer " + testAPIKey} {
		request := httptest.NewRequest("GET", "/metrics", nil)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if token == "Bearer "+testAPIKey {
			if response.Code != http.StatusOK {
				t.Fatalf("authorized metrics status = %d; want 200", response.Code)
			}
		} else {
			assertUnauthorized(t, response)
		}
	}
}

func TestOnlyExplicitPublicRoutesBypassAuthentication(t *testing.T) {
	server := authenticatedTestServer(t, &spyTable{})
	for _, route := range []struct{ method, path string }{
		{"GET", "/"}, {"GET", "/swagger/"}, {"GET", "/swagger/openapi.yaml"},
	} {
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("public %s %s = %d", route.method, route.path, response.Code)
		}
	}
	// Public access matches exact paths and read methods, not path prefixes.
	// Authentication also precedes the mux's path-cleaning redirect.
	for _, route := range []struct{ method, path string }{
		{"POST", "/health"}, {"POST", "/"}, {"GET", "/health/private"},
		{"GET", "/swagger/private"}, {"GET", "/other"}, {"GET", "/keys//name"},
	} {
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		assertUnauthorized(t, response)
	}
}
