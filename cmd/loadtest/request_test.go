package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRunRecordsTransportTimeout(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			return nil, context.DeadlineExceeded
		}
		return localTransport{server}.RoundTrip(r)
	})}
	result := run(client, testConfig())
	if result.Success || len(result.Phases) != 1 {
		t.Fatal("request timeout must fail the run")
	}
	phase := result.Phases[0]
	if phase.Timeouts != 1 || phase.Transport != 1 || phase.Errors != 1 || phase.AverageMS != nil {
		t.Fatalf("incorrect failure counters: %+v", phase)
	}
}

func TestTimedReadAlsoRejectsIncorrectValues(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
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
	if result.Success || len(result.Phases) != 3 || result.Phases[2].InvalidValue != 1 {
		t.Fatal("incorrect values during timed reads must not be accepted")
	}
}

func TestRequestClosesResponseBody(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader("ok")}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
	})}
	status, data, failure, _ := request(client, testConfig(), "GET", "/keys/bench-0", nil, 100)
	if status != 200 || string(data) != "ok" || failure != "" || !body.closed {
		t.Fatal("request must consume and close the response")
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestMetricsRejectsSecretInResponse(t *testing.T) {
	server := &testServer{values: make(map[string]string)}
	cfg := testConfig()
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		response, err := localTransport{server}.RoundTrip(r)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		response.Body = io.NopCloser(strings.NewReader(string(data) + "# " + cfg.APIKey))
		return response, nil
	})}
	result := run(client, cfg)
	if result.Success || len(result.Metrics) != 1 || result.Metrics[0].Text != "" || strings.Contains(result.Failure, cfg.APIKey) {
		t.Fatal("a metrics response exposing the credential must not be saved")
	}
}
