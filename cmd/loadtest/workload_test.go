package main

import (
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPercentilesNearestRank(t *testing.T) {
	values := make([]int64, 100)
	for i := range values {
		values[i] = int64(100-i) * int64(time.Millisecond)
	}
	p50, p95, p99 := percentiles(values)
	if *p50 != 50 || *p95 != 95 || *p99 != 99 || values[0] != int64(100*time.Millisecond) {
		t.Fatal("incorrect nearest-rank values or raw data modified")
	}
	p50, p95, p99 = percentiles(nil)
	if p50 != nil || p95 != nil || p99 != nil {
		t.Fatal("empty sample set must use null percentiles")
	}
}

func TestSeededReadSelection(t *testing.T) {
	calls := func(seed int64) []string {
		server := &testServer{values: make(map[string]string)}
		cfg := testConfig()
		cfg.Count, cfg.Seed = 20, seed
		result := run(&http.Client{Transport: localTransport{server}}, cfg)
		if !result.Success {
			t.Fatal(result.Failure)
		}
		// Measured reads occur after the third collection, before the fourth.
		var selected []string
		collections := 0
		for _, call := range server.calls {
			if call == "GET /metrics" {
				collections++
			} else if collections == 3 {
				selected = append(selected, call)
			}
		}
		return selected
	}
	first := calls(42)
	if len(first) != 20 || !reflect.DeepEqual(first, calls(42)) || reflect.DeepEqual(first, calls(43)) {
		t.Fatal("read selections must repeat for the same seed and change for another seed")
	}
}

func TestIntentionalMissesAreSuccessfulOutcomes(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.MissPercent = 100
	result := run(&http.Client{Transport: localTransport{server}}, cfg)
	read := result.Phases[2]
	misses := read.Operations["GET_miss"]
	if !result.Success || misses == nil || misses.Successes != cfg.Count || misses.Statuses[404] != cfg.Count || read.Errors != 0 {
		t.Fatal("intentional 404 responses must be separate successes, not failures")
	}
	if read.Operations["GET_hit"] != nil || misses.P99MS == nil {
		t.Fatal("missing-key samples must not enter hit percentiles")
	}
}

func TestBoundedConcurrency(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.Count, cfg.Concurrency = 12, 3
	var active, peak atomic.Int32
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); n > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, n) {
				break
			}
		}
		if r.URL.Path != "/metrics" {
			time.Sleep(3 * time.Millisecond)
		}
		return localTransport{server}.RoundTrip(r)
	})}
	result := run(client, cfg)
	if !result.Success || peak.Load() <= 1 || peak.Load() > int32(cfg.Concurrency) {
		t.Fatalf("success=%t peak=%d; expected concurrent execution bounded by %d", result.Success, peak.Load(), cfg.Concurrency)
	}
}

func TestOfferedRateReportsSaturation(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.Rate, cfg.Duration = 1000, 25*time.Millisecond
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/keys/") {
			time.Sleep(5 * time.Millisecond)
		}
		return localTransport{server}.RoundTrip(r)
	})}
	result := run(client, cfg)
	read := result.Phases[2]
	if result.Success || read.Unsent == 0 || read.Scheduled != 25 || read.OfferedRPS != 1000 {
		t.Fatal("an overloaded generator must report unsent slots and a failed rate target")
	}
	if read.Attempts+read.Unsent+read.NotScheduled != read.Scheduled || len(read.ScheduleDelaysNS) != read.Attempts {
		t.Fatal("all arrival slots and dispatched-request delays must be accounted for")
	}
	if read.SuccessfulRPS <= 0 || read.DrainMS < 0 || len(result.Metrics) != 4 {
		t.Fatal("partial rate results and metrics must be preserved")
	}
}

func TestWarmupAndDuration(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.Warmup, cfg.Duration = 12*time.Millisecond, 30*time.Millisecond
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/keys/") {
			time.Sleep(time.Millisecond)
		}
		return localTransport{server}.RoundTrip(r)
	})}
	result := run(client, cfg)
	if !result.Success || len(result.Phases) != 4 {
		t.Fatalf("expected separate warm-up and measured phases: %q", result.Failure)
	}
	warmup, read := result.Phases[2], result.Phases[3]
	if warmup.Name != "warmup" || warmup.Timed || len(warmup.LatenciesNS) != 0 || warmup.Attempts == 0 {
		t.Fatal("warm-up requests must not become measured samples")
	}
	if read.Name != "read" || !read.Timed || read.WindowMS < 25 || read.SuccessfulRPS <= 0 {
		t.Fatal("duration must control the read scheduling window")
	}
	if len(read.LatenciesNS) != read.Attempts || len(result.Metrics) != 4 {
		t.Fatal("read samples and isolated phase snapshots must be retained")
	}
}

func TestPacedCampaignCanSustainItsRate(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.Rate, cfg.Duration = 20, 100*time.Millisecond
	result := run(&http.Client{Transport: localTransport{server}}, cfg)
	if !result.Success {
		t.Fatal(result.Failure)
	}
	read := result.Phases[2]
	if read.Scheduled != 2 || read.Attempts != 2 || read.Unsent != 0 || read.WindowMS < 90 || read.OfferedRPS != 20 {
		t.Fatalf("incorrect paced campaign counters: %+v", read)
	}
}

func TestPacedCountModeStillSendsCountRequests(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.Count, cfg.Rate = 2, 20
	result := run(&http.Client{Transport: localTransport{server}}, cfg)
	if !result.Success || result.Phases[2].Attempts != 2 || result.Phases[2].Unsent != 0 {
		t.Fatal("a paced count-based run must retain the requested count")
	}
}

func TestWorkloadValidation(t *testing.T) {
	for _, change := range []func(*config){
		func(c *config) { c.Concurrency = 0 },
		func(c *config) { c.Concurrency = 257 },
		func(c *config) { c.Rate = -1 },
		func(c *config) { c.Rate = math.NaN() },
		func(c *config) { c.Rate = math.Inf(1) },
		func(c *config) { c.Warmup = -time.Second },
		func(c *config) { c.Duration = -time.Second },
		func(c *config) { c.MissPercent = 101 },
		func(c *config) { c.Rate = 1000000; c.Duration = time.Hour },
	} {
		cfg := testConfig()
		change(&cfg)
		if cfg.validate() == nil {
			t.Fatal("invalid workload parameters accepted")
		}
	}
}

func TestWarmupSchedulingGapsWarnAndContinue(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.Rate, cfg.Warmup, cfg.Duration = 100, 40*time.Millisecond, 100*time.Millisecond
	readsBeforeMeasurement := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if server.collections == 2 && strings.HasPrefix(r.URL.Path, "/keys/") {
			readsBeforeMeasurement++
			if readsBeforeMeasurement > cfg.Count {
				// Slow only warm-up; verification and measured reads remain fast.
				time.Sleep(25 * time.Millisecond)
			}
		}
		return localTransport{server}.RoundTrip(r)
	})}
	result := run(client, cfg)
	if !result.Success || len(result.Phases) != 4 || len(result.Warnings) != 1 {
		t.Fatalf("warm-up scheduling gaps must warn and continue: %q", result.Failure)
	}
	if result.Phases[2].Unsent == 0 || result.Phases[2].Errors != 0 || result.Phases[3].Attempts == 0 || len(result.Metrics) != 4 {
		t.Fatal("warm-up counters and measured results must both be preserved")
	}
}

func TestWarmupDataErrorsRemainFatal(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	cfg.Warmup = 10 * time.Millisecond
	reads := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/keys/") {
			reads++
			if reads > cfg.Count {
				server.corrupt = true
			}
		}
		return localTransport{server}.RoundTrip(r)
	})}
	result := run(client, cfg)
	if result.Success || len(result.Phases) != 3 || !strings.HasPrefix(result.Failure, "warmup:") || result.Phases[2].InvalidValue == 0 {
		t.Fatal("incorrect warm-up values must stop the run before measured reads")
	}
}
