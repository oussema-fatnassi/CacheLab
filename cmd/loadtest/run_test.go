package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The client is exercised over HTTP requests/responses without opening a TCP port.
type localTransport struct{ handler http.Handler }

func (t localTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response := httptest.NewRecorder()
	t.handler.ServeHTTP(response, r)
	return response.Result(), nil
}

type testServer struct {
	mu           sync.Mutex
	values       map[string]string
	calls        []string
	collections  int
	badMetricsAt int
	corrupt      bool
	postStatus   int
}

func (s *testServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	if r.Header.Get("Authorization") != "Bearer unit-test-credential" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/metrics" {
		s.collections++
		if s.collections == s.badMetricsAt {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "cachelab_cache_hits_total 0\ncachelab_cache_misses_total 0\ncachelab_http_errors_total 0\ncachelab_http_request_duration_seconds_count 0\ncachelab_go_memory_sys_bytes 1000\ncachelab_go_heap_alloc_bytes 100\n")
		return
	}
	if r.Method == http.MethodPost {
		var input struct{ Key, Value string }
		if r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&input) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.postStatus != 0 {
			w.WriteHeader(s.postStatus)
			return
		}
		if _, exists := s.values[input.Key]; exists {
			w.WriteHeader(http.StatusConflict)
			return
		}
		s.values[input.Key] = input.Value
		w.WriteHeader(http.StatusCreated)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/keys/")
	value, found := s.values[key]
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if s.corrupt {
		value = "wrong"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"key": key, "value": value})
}

func testConfig() config {
	return config{URL: "http://cache.test", Count: 4, ValueBytes: 100, APIKey: "unit-test-credential", Timeout: time.Second, Label: "test", Concurrency: 1}
}

func TestRunSimpleProtocol(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	result := run(&http.Client{Transport: localTransport{server}}, testConfig())
	if !result.Success || result.Failure != "" || len(result.Phases) != 3 {
		t.Fatalf("unexpected result: success=%t failure=%q", result.Success, result.Failure)
	}
	for _, phase := range result.Phases {
		if phase.Attempts != 4 || phase.Successes != 4 || phase.Errors != 0 {
			t.Fatalf("incorrect counts for %s: %+v", phase.Name, phase)
		}
		if phase.Timed && (phase.AverageMS == nil || len(phase.LatenciesNS) != 4 || phase.TotalMS <= 0) {
			t.Fatalf("missing measurements for %s", phase.Name)
		}
		if !phase.Timed && (phase.AverageMS != nil || phase.TotalMS != 0 || len(phase.LatenciesNS) != 0) {
			t.Fatal("verification must not contribute performance samples")
		}
	}
	if len(result.Metrics) != 4 || result.Metadata["metrics_collected"] != true {
		t.Fatal("expected four complete metrics snapshots")
	}
	want := []string{"GET /metrics"}
	for i := 0; i < 4; i++ {
		want = append(want, "POST /keys")
	}
	want = append(want, "GET /metrics")
	for i := 0; i < 4; i++ {
		want = append(want, fmt.Sprintf("GET /keys/bench-%d", i))
	}
	want = append(want, "GET /metrics")
	// Seed zero produces this reproducible selection with replacement.
	for _, i := range []int{2, 1, 3, 3} {
		want = append(want, fmt.Sprintf("GET /keys/bench-%d", i))
	}
	want = append(want, "GET /metrics")
	if strings.Join(server.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected phase order: %v", server.calls)
	}
	for _, sample := range result.Metrics {
		if sample.CollectedAt.IsZero() || sample.Status != 200 || !validMetrics(sample.Text) {
			t.Fatal("invalid timestamped snapshot")
		}
	}
}

func TestRunRejectsCorruptValues(t *testing.T) {
	server := &testServer{values: make(map[string]string), corrupt: true}
	result := run(&http.Client{Transport: localTransport{server}}, testConfig())
	if result.Success || len(result.Phases) != 2 || result.Phases[1].InvalidValue != 1 {
		t.Fatal("corruption must stop the run before timed reads")
	}
}

func TestRunRejectsFailedMetricsCollection(t *testing.T) {
	for _, collection := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(collection), func(t *testing.T) {
			server := &testServer{values: make(map[string]string), badMetricsAt: collection}
			result := run(&http.Client{Transport: localTransport{server}}, testConfig())
			if result.Success || !strings.Contains(result.Failure, "metrics") || result.Metadata["metrics_collected"] != false {
				t.Fatal("failed collection must not produce a successful report")
			}
		})
	}
}

func TestRunReportsRejectedCreate(t *testing.T) {
	for _, status := range []int{401, 409, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := &testServer{values: make(map[string]string), postStatus: status}
			result := run(&http.Client{Transport: localTransport{server}}, testConfig())
			if result.Success || len(result.Phases) != 1 || result.Phases[0].Statuses[status] != 1 || result.Phases[0].Errors != 1 {
				t.Fatal("unexpected create response must fail immediately and retain its status")
			}
		})
	}
}

func TestRunRequiresAuthentication(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.APIKey = "incorrect-credential"
	result := run(&http.Client{Transport: localTransport{server}}, cfg)
	if result.Success || len(server.values) != 0 || !strings.Contains(result.Failure, "401") {
		t.Fatal("invalid authentication must fail without inserting data")
	}
}

func TestSavedReportContainsNoCredentials(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	result := run(&http.Client{Transport: localTransport{server}}, cfg)
	path := filepath.Join(t.TempDir(), "results", "run.json")
	if err := saveReport(path, result); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), cfg.APIKey) {
		t.Fatal("report contains the API key")
	}
	var decoded report
	if err := json.Unmarshal(data, &decoded); err != nil || !decoded.Success {
		t.Fatal("saved report is not a successful JSON report")
	}
	if err := saveReport(path, result); err == nil {
		t.Fatal("a previous report must not be overwritten")
	}
}

func TestValidMetricsRejectsInvalidExport(t *testing.T) {
	for _, text := range []string{"", "<html>not metrics</html>", "cachelab_go_memory_sys_bytes NaN\n"} {
		if validMetrics(text) {
			t.Fatal("invalid export was accepted")
		}
	}
}

func TestConfigValidation(t *testing.T) {
	for _, mutate := range []func(*config){
		func(c *config) { c.Count = 0 },
		func(c *config) { c.Count = 1000001 },
		func(c *config) { c.ValueBytes = 0 },
		func(c *config) { c.Timeout = 0 },
		func(c *config) { c.APIKey = "" },
		func(c *config) { c.APIKey = "secret\nheader" },
		func(c *config) { c.URL = "http://user:secret@host" },
		func(c *config) { c.URL = "http://host?token=secret" },
		func(c *config) { c.URL = "http://host/path" },
		func(c *config) { c.Count = 1000000; c.ValueBytes = 1048576 },
	} {
		cfg := testConfig()
		mutate(&cfg)
		if cfg.validate() == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
