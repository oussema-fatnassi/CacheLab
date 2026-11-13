package main

import (
	"errors"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestCounterDuration(t *testing.T) {
	for _, test := range []struct {
		ticks, frequency int64
		want             time.Duration
	}{
		{0, 10000000, 0},
		{1, 10000000, 100 * time.Nanosecond},
		{10000000, 10000000, time.Second},
		{12500001, 10000000, time.Second + 250*time.Millisecond + 100*time.Nanosecond},
		{1, 3, 333333333 * time.Nanosecond},
		// The fractional multiplication overflows uint64 without 128-bit arithmetic.
		{1<<63 - 2, 1<<63 - 1, time.Second - time.Nanosecond},
	} {
		got, err := counterDuration(test.ticks, test.frequency)
		if err != nil || got != test.want {
			t.Fatalf("counterDuration(%d, %d) = %v, %v; want %v", test.ticks, test.frequency, got, err, test.want)
		}
	}
	for _, test := range [][2]int64{{-1, 10}, {1, 0}, {1, -1}, {1<<63 - 1, 1}} {
		if _, err := counterDuration(test[0], test[1]); err == nil {
			t.Fatal("invalid or overflowing duration must fail")
		}
	}
}

func TestLatencyClockAdvances(t *testing.T) {
	clock, err := newLatencyClock()
	if err != nil {
		t.Fatal(err)
	}
	if clock.name == "" || clock.frequency <= 0 {
		t.Fatal("clock source and frequency must be available")
	}
	if runtime.GOOS == "windows" && clock.name != "Windows QueryPerformanceCounter" {
		t.Fatal("Windows latency measurements must use QPC")
	}
	start, err := clock.now()
	if err != nil {
		t.Fatal(err)
	}
	previous := start
	for i := 0; i < 1000; i++ {
		current, err := clock.now()
		if err != nil || current < previous {
			t.Fatal("measurement clock failed or moved backwards")
		}
		previous = current
	}
	if previous <= start {
		t.Fatal("clock did not advance during repeated reads")
	}
}

func TestLatencyClockRejectsBackwardsReading(t *testing.T) {
	clock := latencyClock{now: func() (time.Duration, error) { return time.Nanosecond, nil }}
	if _, err := clock.since(2 * time.Nanosecond); err == nil {
		t.Fatal("backwards readings must fail instead of producing invalid samples")
	}
}

func TestClockFailureCannotBecomeZeroLatency(t *testing.T) {
	var readings atomic.Int32
	clock := latencyClock{now: func() (time.Duration, error) {
		if readings.Add(1) == 1 {
			return 0, nil
		}
		return 0, errors.New("test clock failure")
	}}
	var requests atomic.Int32
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("unexpected request")
	})}
	cfg := testConfig()
	cfg.Count = 1
	phase, failure := executePhase(client, cfg, []entry{{Key: "bench-0"}}, "x", "read", clock)
	if failure == "" || phase.TimingErrors == 0 || len(phase.LatenciesNS) != 0 || requests.Load() != 0 {
		t.Fatal("clock errors must fail the phase without inventing zero samples")
	}
}
