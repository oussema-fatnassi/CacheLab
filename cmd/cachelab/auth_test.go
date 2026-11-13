package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
	server := authenticatedTestServer(t, cache.NewHashTable(16))
	for _, step := range []struct {
		method, path, body, wantBody string
		want                         int
	}{
		{"POST", "/keys", `{"key":"name","value":"Test"}`, "", 201},
		{"GET", "/keys/name", "", `{"key":"name","value":"Test"}`, 200},
		{"PUT", "/keys/name", `{"value":"Alice"}`, "", 200},
		{"GET", "/keys/name", "", `{"key":"name","value":"Alice"}`, 200},
		{"DELETE", "/keys/name", "", "", 204},
		{"GET", "/keys/name", "", `{"error":"key not found"}`, 404},
	} {
		request := httptest.NewRequest(step.method, step.path, strings.NewReader(step.body))
		request.Header.Set("Authorization", "bEaReR "+testAPIKey)
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != step.want || strings.TrimSpace(response.Body.String()) != step.wantBody {
			t.Fatalf("%s %s = %d %q; want %d %q", step.method, step.path, response.Code, response.Body.String(), step.want, step.wantBody)
		}
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

func TestMetricsIsProtectedBeforeExporterExists(t *testing.T) {
	server := authenticatedTestServer(t, &spyTable{})
	for _, token := range []string{"", "Bearer wrong-key", "Bearer " + testAPIKey} {
		request := httptest.NewRequest("GET", "/metrics", nil)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		if token == "Bearer "+testAPIKey {
			if response.Code != http.StatusNotFound {
				t.Fatalf("authorized metrics status = %d; want 404 until OBS-01", response.Code)
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
	for _, route := range []struct{ method, path string }{
		{"POST", "/health"}, {"POST", "/"}, {"GET", "/health/private"},
		{"GET", "/swagger/private"}, {"GET", "/other"}, {"GET", "/keys//name"},
	} {
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		assertUnauthorized(t, response)
	}
}
