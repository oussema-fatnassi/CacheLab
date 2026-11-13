package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// failOnKey stores entries in a real HashTable but fails to create the key "boom".
type failOnKey struct{ *cache.HashTable }

func (t failOnKey) Create(key, value string) error {
	if key == "boom" {
		return errors.New("storage failure")
	}
	return t.HashTable.Create(key, value)
}

// metricsHandler returns the real server handler, with logs discarded.
func metricsHandler() http.Handler {
	service := cache.NewCacheService(failOnKey{cache.NewHashTable(16)}, 64, 64)
	return serverHandler(serverConfig{apiKey: testAPIKey, maxBodyBytes: 64}, service, slog.New(slog.DiscardHandler))
}

// scrapeMetrics reads /metrics with a valid key and returns its text.
func scrapeMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authorizedRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status = %d; want 200", recorder.Code)
	}
	return recorder.Body.String()
}

// assertMetricLines fails unless every line appears exactly in the export.
func assertMetricLines(t *testing.T, text string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(text, line+"\n") {
			t.Fatalf("metrics miss %q:\n%s", line, text)
		}
	}
}

func TestMetricsCountKnownRequestSequence(t *testing.T) {
	handler := metricsHandler()
	requests := []*http.Request{
		authorizedRequest(http.MethodPost, "/keys", strings.NewReader(`{"key": "a", "value": "1"}`)),    // 201
		authorizedRequest(http.MethodGet, "/keys/a", nil),                                               // 200: hit
		authorizedRequest(http.MethodGet, "/keys/a", nil),                                               // 200: hit
		authorizedRequest(http.MethodGet, "/keys/none", nil),                                            // 404: miss
		authorizedRequest(http.MethodPost, "/keys", strings.NewReader(`{"key": "boom", "value": "1"}`)), // 500: error
		httptest.NewRequest(http.MethodGet, "/keys/a", nil),                                             // 401: refused
	}
	for _, request := range requests {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}

	assertMetricLines(t, scrapeMetrics(t, handler),
		`cachelab_http_requests_total{route="GET /keys/{key}",status="200"} 2`,
		`cachelab_http_requests_total{route="GET /keys/{key}",status="401"} 1`,
		`cachelab_http_requests_total{route="GET /keys/{key}",status="404"} 1`,
		`cachelab_http_requests_total{route="POST /keys",status="201"} 1`,
		`cachelab_http_requests_total{route="POST /keys",status="500"} 1`,
		"cachelab_cache_hits_total 2",
		"cachelab_cache_misses_total 1",
		// The miss is a normal answer: only the storage failure is an internal error.
		"cachelab_http_errors_total 1",
		`cachelab_http_request_duration_seconds_bucket{le="+Inf"} 6`,
		"cachelab_http_request_duration_seconds_count 6",
	)
}

func TestMetricsDocumentUnitsAndBoundaries(t *testing.T) {
	text := scrapeMetrics(t, metricsHandler())
	assertMetricLines(t, text,
		"# TYPE cachelab_http_requests_total counter",
		"# TYPE cachelab_cache_hits_total counter",
		"# TYPE cachelab_cache_misses_total counter",
		"# TYPE cachelab_http_errors_total counter",
		"# TYPE cachelab_http_request_duration_seconds histogram",
		`cachelab_http_request_duration_seconds_bucket{le="0.0001"} 0`,
		`cachelab_http_request_duration_seconds_bucket{le="1"} 0`,
		"# TYPE cachelab_go_memory_sys_bytes gauge",
		"# TYPE cachelab_go_heap_alloc_bytes gauge",
	)
	// Every exported metric has a description stating its meaning and unit.
	if strings.Count(text, "# HELP ") != strings.Count(text, "# TYPE ") {
		t.Fatalf("every metric needs a HELP line:\n%s", text)
	}
}

func TestMetricsHideKeysValuesAndSecrets(t *testing.T) {
	handler := metricsHandler()
	wrongKey := httptest.NewRequest(http.MethodGet, "/keys/user:42:email", nil)
	wrongKey.Header.Set("Authorization", "Bearer wrong-secret-key")
	for _, request := range []*http.Request{
		authorizedRequest(http.MethodPost, "/keys", strings.NewReader(`{"key": "user:42:email", "value": "alice@example.com"}`)),
		authorizedRequest(http.MethodGet, "/keys/user:42:email", nil),
		wrongKey,
	} {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}

	text := scrapeMetrics(t, handler)
	for _, secret := range []string{"user:42", "alice@example.com", testAPIKey, "wrong-secret-key"} {
		if strings.Contains(text, secret) {
			t.Fatalf("metrics leak %q:\n%s", secret, text)
		}
	}
}

func TestMetricsAreSafeUnderConcurrency(t *testing.T) {
	handler := metricsHandler()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			key := fmt.Sprintf("k%d", i)
			handler.ServeHTTP(httptest.NewRecorder(), authorizedRequest(http.MethodPost, "/keys", strings.NewReader(`{"key": "`+key+`", "value": "v"}`)))
			handler.ServeHTTP(httptest.NewRecorder(), authorizedRequest(http.MethodGet, "/keys/"+key, nil))
		})
	}
	wg.Wait()

	assertMetricLines(t, scrapeMetrics(t, handler),
		`cachelab_http_requests_total{route="POST /keys",status="201"} 50`,
		`cachelab_http_requests_total{route="GET /keys/{key}",status="200"} 50`,
		"cachelab_cache_hits_total 50",
		"cachelab_http_request_duration_seconds_count 100",
	)
}

// BenchmarkMetricsObserve measures the cost that the counters add to each request.
func BenchmarkMetricsObserve(b *testing.B) {
	metrics := newServerMetrics()
	for b.Loop() {
		metrics.observe("GET /keys/{key}", http.StatusOK, 150*time.Microsecond)
	}
}
