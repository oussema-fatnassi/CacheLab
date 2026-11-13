package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type report struct {
	Metadata map[string]any   `json:"metadata"`
	Success  bool             `json:"success"`
	Failure  string           `json:"failure,omitempty"`
	Phases   []phaseResult    `json:"phases"`
	Metrics  []metricSnapshot `json:"metrics"`
}

type phaseResult struct {
	Name               string                     `json:"name"`
	Timed              bool                       `json:"timed"`
	Attempts           int                        `json:"attempts"`
	Successes          int                        `json:"successes"`
	Errors             int                        `json:"errors"`
	Statuses           map[int]int                `json:"http_statuses"`
	Transport          int                        `json:"transport_errors"`
	ResponseReadErrors int                        `json:"response_read_errors"`
	TimingErrors       int                        `json:"timing_errors"`
	Timeouts           int                        `json:"timeouts"`
	InvalidValue       int                        `json:"invalid_values"`
	TotalMS            float64                    `json:"total_ms"`
	AverageMS          *float64                   `json:"average_ms,omitempty"`
	LatenciesNS        []int64                    `json:"latencies_ns,omitempty"`
	Operations         map[string]*operationStats `json:"operations,omitempty"`
	Scheduled          int                        `json:"scheduled"`
	Unsent             int                        `json:"unsent"`
	NotScheduled       int                        `json:"not_scheduled_after_abort"`
	Delayed            int                        `json:"delayed_over_1ms"`
	ScheduleDelaysNS   []int64                    `json:"schedule_delays_ns,omitempty"`
	OfferedRPS         float64                    `json:"offered_rps"`
	StartedRPS         float64                    `json:"dispatched_rps"`
	SuccessfulRPS      float64                    `json:"successful_rps"`
	CompletedRPS       float64                    `json:"completed_response_rps"`
	WindowMS           float64                    `json:"scheduling_window_ms"`
	DrainMS            float64                    `json:"drain_ms"`
}

type entry struct {
	Key  string
	Body []byte
}

// run separates generation, insertion, exhaustive verification, warm-up and reads.
func run(client *http.Client, cfg config) report {
	if err := cfg.validate(); err != nil {
		return report{Failure: err.Error()}
	}
	result := report{Metadata: metadata(cfg)}
	clock, err := newLatencyClock()
	if err != nil {
		result.Failure = "measurement clock initialization failed: " + err.Error()
		return result
	}
	result.Metadata["latency_clock"] = clock.name
	result.Metadata["latency_clock_frequency_hz"] = clock.frequency
	value := strings.Repeat("x", cfg.ValueBytes)
	entries := make([]entry, cfg.Count)
	for i := range entries {
		key := fmt.Sprintf("bench-%d", i)
		body, err := json.Marshal(map[string]string{"key": key, "value": value})
		if err != nil {
			result.Failure = "could not generate the dataset"
			return result
		}
		entries[i] = entry{Key: key, Body: body}
	}

	phases := []string{"insert", "verify"}
	if cfg.Warmup > 0 {
		phases = append(phases, "warmup")
	}
	phases = append(phases, "read")
	for _, name := range phases {
		timed := name == "insert" || name == "read"
		if timed && !collectMetrics(client, cfg, &result, name, "before") {
			return result
		}
		phase, failure := executePhase(client, cfg, entries, value, name, clock)
		result.Phases = append(result.Phases, phase)
		if failure != "" {
			result.Failure = name + ": " + failure
		}
		if timed && !collectMetrics(client, cfg, &result, name, "after") {
			return result
		}
		if result.Failure != "" {
			return result
		}
	}
	result.Success = true
	result.Metadata["metrics_collected"] = true
	return result
}

// request consumes and closes each response so HTTP keep-alive can reuse connections.
// Error details and response bodies are never copied into the report.
func request(client *http.Client, cfg config, method, path string, body []byte, limit int64) (int, []byte, string, bool) {
	req, err := http.NewRequest(method, strings.TrimRight(cfg.URL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, "could not create request", false
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil, "request failed", os.IsTimeout(err)
	}
	// Bound unexpected response sizes, including error responses.
	data, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || int64(len(data)) > limit {
		return response.StatusCode, nil, "response could not be read completely", os.IsTimeout(readErr)
	}
	return response.StatusCode, data, "", false
}

func writeReportFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	// Keep previous results instead of silently overwriting them.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
