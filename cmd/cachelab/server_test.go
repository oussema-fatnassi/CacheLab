package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerConfigurationRejectsInvalidSettings(t *testing.T) {
	for _, s := range []struct{ name, value string }{{"PORT", "65536"}, {"PORT", "abc"}, {"MAX_BODY_BYTES", "0"}, {"SHUTDOWN_TIMEOUT", "invalid"}, {"SHUTDOWN_TIMEOUT", "-1s"}} {
		t.Run(s.name+"="+s.value, func(t *testing.T) {
			t.Setenv(s.name, s.value)
			if _, err := loadServerConfig(); err == nil {
				t.Fatal("expected invalid configuration")
			}
		})
	}
}

func TestRoutesAndBodyLimit(t *testing.T) {
	called := make(chan string, 4)
	handler := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called <- name; w.WriteHeader(http.StatusNoContent) })
	}
	mux := registerRoutes(routeHandlers{handler("create"), handler("read"), handler("update"), handler("delete")}, 5)
	for _, tc := range []struct{ method, path, body, want string }{{"POST", "/keys", "12345", "create"}, {"GET", "/keys/a", "", "read"}, {"PUT", "/keys/a", "12345", "update"}, {"DELETE", "/keys/a", "", "delete"}} {
		req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		w := &responseRecorder{header: make(http.Header)}
		mux.ServeHTTP(w, req)
		if w.status != http.StatusNoContent || <-called != tc.want {
			t.Fatalf("%s %s routed incorrectly", tc.method, tc.path)
		}
	}
	req, _ := http.NewRequest("POST", "/keys", strings.NewReader("123456"))
	req.ContentLength = -1
	w := &responseRecorder{header: make(http.Header)}
	mux.ServeHTTP(w, req)
	if w.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.status)
	}
}

type responseRecorder struct {
	header http.Header
	status int
}

func (w *responseRecorder) Header() http.Header    { return w.header }
func (w *responseRecorder) WriteHeader(status int) { w.status = status }
func (w *responseRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return len(p), nil
}

func TestServerShutdownWaitsForRequests(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		if _, err := io.WriteString(w, "done"); err != nil {
			t.Errorf("writing response: %v", err)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &CacheServer{httpServer: &http.Server{Handler: handler}}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	response := make(chan error, 1)
	go func() {
		r, e := http.Get("http://" + listener.Addr().String())
		if e == nil {
			e = r.Body.Close()
		}
		response <- e
	}()
	<-started
	shutdown := make(chan error, 1)
	go func() { shutdown <- server.Shutdown(context.Background()) }()
	select {
	case err := <-shutdown:
		t.Fatalf("shutdown returned before request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServerShutdownTimeout(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &CacheServer{httpServer: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release })}}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	response := make(chan error, 1)
	go func() {
		r, e := http.Get("http://" + listener.Addr().String())
		if e == nil {
			e = r.Body.Close()
		}
		response <- e
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := server.Shutdown(ctx); err != context.DeadlineExceeded {
		t.Fatalf("shutdown error = %v", err)
	}
	close(release)

	if err := <-response; err != nil {
		t.Error(err)
	}
	if err := <-done; err != nil {
		t.Error(err)
	}
}

func TestServerListenReportsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	}()
	server := &CacheServer{httpServer: &http.Server{Addr: listener.Addr().String()}}
	if err := server.ListenAndServe(); err == nil {
		t.Fatal("expected occupied port error")
	}
}

func TestServerConfigurationAllowsSmallBodyLimit(t *testing.T) {
	t.Setenv("MAX_BODY_BYTES", "10")
	if _, err := loadServerConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestServerKeepsExistingRootHandlerWithoutCRUDHandlers(t *testing.T) {
	server := newCacheServer(serverConfig{maxBodyBytes: 100}, nil)
	req := httptest.NewRequest(http.MethodPost, "/other", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Body.String() != "Hello, World!" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestServerCreateRoute(t *testing.T) {
	app, err := newApplication()
	if err != nil {
		t.Fatal(err)
	}
	server := newCacheServer(serverConfig{maxBodyBytes: 64}, app.service)
	request := httptest.NewRequest(http.MethodPost, "/keys", strings.NewReader(`{"key":"name","value":"Test"}`))
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d; want 201", response.Code)
	}
	if value, err := app.service.Read("name"); err != nil || value != "Test" {
		t.Fatalf("Read = %q, %v", value, err)
	}
	request = httptest.NewRequest(http.MethodPost, "/keys", strings.NewReader(strings.Repeat("x", 65)))
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d; want 413", response.Code)
	}
}

func TestServerReadRoute(t *testing.T) {
	app, err := newApplication()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.service.Create("name", "Test"); err != nil {
		t.Fatal(err)
	}
	server := newCacheServer(serverConfig{maxBodyBytes: 64}, app.service)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/keys/name", nil))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"key":"name","value":"Test"}` {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/keys/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing key status = %d; want 404", response.Code)
	}
}

func TestServerUpdateRoute(t *testing.T) {
	app, err := newApplication()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.service.Create("name", "Test"); err != nil {
		t.Fatal(err)
	}
	server := newCacheServer(serverConfig{maxBodyBytes: 64}, app.service)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/keys/name", strings.NewReader(`{"value":"Alice"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", response.Code)
	}
	// Integration: reading the key afterwards returns the new value.
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/keys/name", nil))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"key":"name","value":"Alice"}` {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestServerDeleteRoute(t *testing.T) {
	app, err := newApplication()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.service.Create("name", "Test"); err != nil {
		t.Fatal(err)
	}
	server := newCacheServer(serverConfig{maxBodyBytes: 64}, app.service)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/keys/name", nil))
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("response = %d %q; want 204 with no body", response.Code, response.Body.String())
	}
	// Integration: reading the key afterwards reports it as absent.
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/keys/name", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("read after delete status = %d; want 404", response.Code)
	}
}
