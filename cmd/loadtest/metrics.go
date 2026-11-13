package main

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type metricSnapshot struct {
	Phase       string    `json:"phase"`
	Position    string    `json:"position"`
	CollectedAt time.Time `json:"collected_at_utc"`
	Status      int       `json:"http_status"`
	Text        string    `json:"prometheus_text,omitempty"`
}

// collectMetrics runs outside operation timers. Failed collection invalidates the run.
func collectMetrics(client *http.Client, cfg config, result *report, phase, position string) bool {
	status, body, failure, _ := request(client, cfg, http.MethodGet, "/metrics", nil, 4*1024*1024)
	snapshot := metricSnapshot{Phase: phase, Position: position, CollectedAt: time.Now().UTC(), Status: status}
	if failure == "" && status != http.StatusOK {
		failure = fmt.Sprintf("unexpected HTTP status %d", status)
	}
	if failure == "" && (!validMetrics(string(body)) || strings.Contains(string(body), cfg.APIKey)) {
		failure = "invalid metrics export"
	}
	if failure == "" {
		snapshot.Text = string(body)
	}
	result.Metrics = append(result.Metrics, snapshot)
	if failure != "" {
		message := "metrics " + phase + " " + position + ": " + failure
		if result.Failure != "" {
			result.Failure += "; " + message
		} else {
			result.Failure = message
		}
		return false
	}
	return true
}

// Check the baseline scalar metrics instead of accepting an arbitrary 200 page.
func validMetrics(text string) bool {
	required := map[string]bool{
		"cachelab_cache_hits_total":                    false,
		"cachelab_cache_misses_total":                  false,
		"cachelab_http_errors_total":                   false,
		"cachelab_http_request_duration_seconds_count": false,
		"cachelab_go_memory_sys_bytes":                 false,
		"cachelab_go_heap_alloc_bytes":                 false,
	}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if _, wanted := required[fields[0]]; wanted {
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return false
			}
			required[fields[0]] = true
		}
	}
	for _, present := range required {
		if !present {
			return false
		}
	}
	return true
}
