package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"sync"
	"time"
)

const maxSamples = 1000000

type operationStats struct {
	Attempts    int         `json:"attempts"`
	Successes   int         `json:"successes"`
	Errors      int         `json:"errors"`
	Statuses    map[int]int `json:"http_statuses"`
	LatenciesNS []int64     `json:"successful_latencies_ns"`
	P50MS       *float64    `json:"p50_ms"`
	P95MS       *float64    `json:"p95_ms"`
	P99MS       *float64    `json:"p99_ms"`
}

type workItem struct {
	entry
	missing bool
	due     time.Time
}

func (c config) validateWorkload() error {
	if c.Concurrency < 1 || c.Concurrency > 256 {
		return fmt.Errorf("concurrency must be 1..256")
	}
	if math.IsNaN(c.Rate) || math.IsInf(c.Rate, 0) || c.Rate < 0 || c.Rate > 1000000 {
		return fmt.Errorf("rate must be finite and between 0 and 1000000 requests/second")
	}
	if c.Warmup < 0 || c.Duration < 0 || c.Warmup > time.Hour || c.Duration > time.Hour || c.MissPercent < 0 || c.MissPercent > 100 {
		return fmt.Errorf("warmup/duration must be 0..1h and miss-percent 0..100")
	}
	if c.Rate*max(c.Warmup, c.Duration).Seconds() > maxSamples {
		return fmt.Errorf("rate multiplied by duration/warmup must not exceed 1000000 scheduled requests")
	}
	return nil
}

// executePhase bounds all in-flight requests. A single scheduler owns the seeded
// generator; workers never generate randomness, so scheduling order is reproducible.
func executePhase(client *http.Client, cfg config, entries []entry, value, name string) (phaseResult, string) {
	phase := phaseResult{Name: name, Timed: name == "insert" || name == "read", Statuses: make(map[int]int)}
	if phase.Timed {
		phase.Operations = make(map[string]*operationStats)
	}
	var duration time.Duration
	if name == "read" {
		duration = cfg.Duration
	} else if name == "warmup" {
		duration = cfg.Warmup
	}
	rate := 0.0
	if name == "read" || name == "warmup" {
		rate = cfg.Rate
	}
	phase.OfferedRPS = rate
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := make(chan workItem, cfg.Concurrency)
	slots := make(chan struct{}, cfg.Concurrency)
	var workers sync.WaitGroup
	var mu sync.Mutex
	failure := ""
	started := time.Now()
	for i := 0; i < cfg.Concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				method, path, want, group := http.MethodGet, "/keys/"+item.Key, http.StatusOK, "GET_hit"
				var body []byte
				if name == "insert" {
					method, path, want, group, body = http.MethodPost, "/keys", http.StatusCreated, "POST", item.Body
				} else if item.missing {
					path, want, group = "/keys/miss-"+item.Key, http.StatusNotFound, "GET_miss"
				}
				requestStart := time.Now()
				status, response, problem, timeout := request(client, cfg, method, path, body, int64(cfg.ValueBytes)*6+1024)
				elapsed := time.Since(requestStart)
				responseReadFailed := problem != "" && status != 0
				invalidValue := false
				if problem == "" {
					if status != want {
						problem = fmt.Sprintf("unexpected HTTP status %d (expected %d)", status, want)
					} else if method == http.MethodGet && !item.missing {
						var stored struct{ Key, Value string }
						if json.Unmarshal(response, &stored) != nil || stored.Key != item.Key || stored.Value != value {
							invalidValue = true
							problem = "incorrect or missing stored value"
						}
					}
				}
				mu.Lock()
				phase.Attempts++
				if status != 0 {
					phase.Statuses[status]++
				}
				if status == 0 {
					phase.Transport++
				}
				if timeout {
					phase.Timeouts++
				}
				if responseReadFailed {
					phase.ResponseReadErrors++
				}
				if invalidValue {
					phase.InvalidValue++
				}
				if problem == "" {
					phase.Successes++
				} else {
					phase.Errors++
					if failure == "" {
						failure = problem
						cancel() // Stop scheduling; already-sent requests still drain.
					}
				}
				if phase.Timed {
					phase.LatenciesNS = append(phase.LatenciesNS, elapsed.Nanoseconds())
					if rate > 0 {
						delay := max(int64(0), requestStart.Sub(item.due).Nanoseconds())
						phase.ScheduleDelaysNS = append(phase.ScheduleDelaysNS, delay)
						if delay > int64(time.Millisecond) {
							phase.Delayed++
						}
					}
					stats := phase.Operations[group]
					if stats == nil {
						stats = &operationStats{Statuses: make(map[int]int)}
						phase.Operations[group] = stats
					}
					stats.Attempts++
					if status != 0 {
						stats.Statuses[status]++
					}
					if problem == "" {
						stats.Successes++
						stats.LatenciesNS = append(stats.LatenciesNS, elapsed.Nanoseconds())
					} else {
						stats.Errors++
					}
				}
				mu.Unlock()
				<-slots
			}
		}()
	}
	planned, unsent, schedulerFailure := schedule(ctx, jobs, slots, cfg, entries, name, started, duration, rate)
	schedulingEnd := time.Now()
	close(jobs)
	workers.Wait()
	finished := time.Now()
	phase.Scheduled, phase.Unsent = planned, unsent
	phase.NotScheduled = max(0, planned-phase.Attempts-unsent)
	phase.WindowMS = float64(schedulingEnd.Sub(started)) / float64(time.Millisecond)
	phase.DrainMS = float64(finished.Sub(schedulingEnd)) / float64(time.Millisecond)
	if failure == "" {
		failure = schedulerFailure
	}
	if failure == "" && rate > 0 && phase.NotScheduled > 0 {
		failure = "generator ended before all planned arrival slots were scheduled"
	}
	if failure == "" && unsent > 0 {
		failure = "offered rate not sustained: some requests were not sent"
	}
	if phase.Timed {
		elapsed := finished.Sub(started)
		phase.TotalMS = float64(elapsed) / float64(time.Millisecond)
		if elapsed > 0 {
			phase.SuccessfulRPS = float64(phase.Successes) / elapsed.Seconds()
			phase.CompletedRPS = float64(phase.Attempts-phase.Transport-phase.ResponseReadErrors) / elapsed.Seconds()
		}
		if schedulingEnd.After(started) {
			phase.StartedRPS = float64(phase.Attempts) / schedulingEnd.Sub(started).Seconds()
		}
		if phase.Errors == 0 && phase.Attempts > 0 {
			var sum float64
			for _, latency := range phase.LatenciesNS {
				sum += float64(latency) / float64(time.Millisecond)
			}
			average := sum / float64(phase.Attempts)
			phase.AverageMS = &average
		}
		for _, group := range phase.Operations {
			group.P50MS, group.P95MS, group.P99MS = percentiles(group.LatenciesNS)
		}
	}
	return phase, failure
}

// schedule implements either a bounded closed loop (rate=0) or independent
// arrival slots (rate>0). Busy arrival slots are counted, never queued unboundedly.
func schedule(ctx context.Context, jobs chan<- workItem, slots chan struct{}, cfg config, entries []entry, name string, started time.Time, duration time.Duration, rate float64) (int, int, string) {
	window := ctx
	if duration > 0 {
		var cancel context.CancelFunc
		window, cancel = context.WithDeadline(ctx, started.Add(duration))
		defer cancel()
	}
	planned := cfg.Count
	if duration > 0 {
		if rate > 0 {
			planned = int(math.Ceil(duration.Seconds() * rate))
		} else {
			planned = maxSamples
		}
	}
	rng := rand.New(rand.NewSource(cfg.Seed))
	unsent := 0
	for i := 0; i < planned; i++ {
		if window.Err() != nil {
			if rate == 0 && duration > 0 {
				return i, unsent, ""
			}
			return planned, unsent, ""
		}
		item := workItem{entry: entries[i%len(entries)]}
		if name == "read" || name == "warmup" {
			item.entry = entries[rng.Intn(len(entries))]
			item.missing = rng.Intn(100) < cfg.MissPercent
		}
		if rate > 0 {
			item.due = started.Add(time.Duration(float64(i) / rate * float64(time.Second)))
			if !waitUntil(window, item.due) {
				return planned, unsent, ""
			}
			select {
			case slots <- struct{}{}:
				jobs <- item
			default:
				unsent++
			}
		} else {
			select {
			case slots <- struct{}{}:
				jobs <- item
			case <-window.Done():
				if duration > 0 {
					return i, unsent, ""
				}
				return planned, unsent, ""
			}
		}
	}
	if duration > 0 && rate == 0 {
		return planned, unsent, "sample limit reached before the requested duration"
	}
	if rate > 0 {
		end := started.Add(time.Duration(float64(planned) / rate * float64(time.Second)))
		if duration > 0 {
			end = started.Add(duration)
		}
		waitUntil(ctx, end)
	}
	return planned, unsent, ""
}

func waitUntil(ctx context.Context, deadline time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	if delay := time.Until(deadline); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return false
		}
	}
	return ctx.Err() == nil
}

// Nearest-rank percentiles use successful response samples only. Error/timeout
// counts remain visible separately; no samples means null, never a fake zero.
func percentiles(latencies []int64) (*float64, *float64, *float64) {
	if len(latencies) == 0 {
		return nil, nil, nil
	}
	ordered := slices.Clone(latencies)
	slices.Sort(ordered)
	at := func(p float64) *float64 {
		value := float64(ordered[int(math.Ceil(p*float64(len(ordered))))-1]) / float64(time.Millisecond)
		return &value
	}
	return at(0.50), at(0.95), at(0.99)
}
