package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestCacheIsEmptyAfterProcessRestart checks volatility with a real restart (MEM-06):
// it runs the shared HTTP scenario, then restarts each configured engine and
// checks that its data is gone. All engines run sequentially from one binary.
func TestCacheIsEmptyAfterProcessRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and starts real server processes")
	}
	binary := buildServer(t)
	for _, engine := range []string{"chained", "recursive", "go-map"} {
		t.Run(engine, func(t *testing.T) {
			port := freePort(t)
			baseURL := "http://127.0.0.1:" + port
			client := &http.Client{Timeout: 2 * time.Second}
			t.Cleanup(client.CloseIdleConnections)

			first := startServer(t, binary, port, engine)
			runConfiguredHTTPConformance(t, client, baseURL, testAPIKey)
			expectHTTP(t, client, baseURL, "POST", "/keys", `{"key":"old","value":"v"}`, 201, "")
			expectHTTP(t, client, baseURL, "GET", "/keys/old", "", 200, `{"key":"old","value":"v"}`)
			stopServer(t, first)
			assertProcessMetadata(t, first, engine)
			client.CloseIdleConnections()

			second := startServer(t, binary, port, engine)
			// The old key is absent; the same CRUD scenario works on a fresh cache.
			expectHTTP(t, client, baseURL, "GET", "/keys/old", "", 404, `{"error":"key not found"}`)
			runAuthorizedCRUD(t, client, baseURL, testAPIKey)
			stopServer(t, second)
			assertProcessMetadata(t, second, engine)
		})
	}
}

func TestServerRejectsUnknownEngineBeforeServing(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and starts a real server process")
	}
	binary := buildServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), serverTestEnvironment("unknown-sensitive-value", freePort(t))...)
	output, err := cmd.CombinedOutput()
	var exitError *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exitError) {
		t.Fatalf("expected configuration exit before timeout: %v, %v\n%s", err, ctx.Err(), output)
	}
	if !bytes.Contains(output, []byte("CACHE_ENGINE must be")) || bytes.Contains(output, []byte("Listening on")) || bytes.Contains(output, []byte("execution metadata")) {
		t.Fatalf("unexpected startup output:\n%s", output)
	}
	if bytes.Contains(output, []byte("unknown-sensitive-value")) || bytes.Contains(output, []byte(testAPIKey)) {
		t.Fatal("startup error exposed environment values")
	}
}

// buildServer compiles this package once per parent test without -race. The
// configured in-process HTTP and shared engine suites provide race coverage.
func buildServer(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "cachelab")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}
	return binary
}

// freePort asks the system for a port that is free right now.
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// serverProcess is a running server and everything it has written.
type serverProcess struct {
	cmd    *exec.Cmd
	output *bytes.Buffer
}

// startServer runs the binary with a test API key and waits until /health answers.
func startServer(t *testing.T, binary, port, engine string) serverProcess {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), serverTestEnvironment(engine, port)...)
	output := &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	server := serverProcess{cmd: cmd, output: output}
	t.Cleanup(func() { stopServer(t, server) })
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()

	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		response, err := client.Get("http://127.0.0.1:" + port + "/health")
		if err == nil {
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode == http.StatusOK {
				return server
			}
		}
	}
	stopServer(t, server)
	t.Fatalf("server did not start:\n%s", output)
	return server
}

// stopServer ends the process. Kill works on every system: the process memory,
// and the cache with it, is released by the system as in any other stop.
func stopServer(t *testing.T, server serverProcess) {
	t.Helper()
	if server.cmd.ProcessState != nil {
		return // already stopped
	}
	if err := server.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("stopping server: %v", err)
	}
	// Wait reports the kill as an error; it only releases the process resources.
	_ = server.cmd.Wait()
}

// Logs are inspected only after Wait, when the subprocess has stopped writing.
func assertProcessMetadata(t *testing.T, server serverProcess, engine string) {
	t.Helper()
	metadataCount, requestCount := 0, 0
	for _, event := range logEvents(t, server.output) {
		switch event["msg"] {
		case "execution metadata":
			metadataCount++
			if event["engine"] != engine || event["server_go_version"] != runtime.Version() {
				t.Fatalf("unexpected process metadata: %v", event)
			}
		case "http request":
			requestCount++
		}
	}
	if metadataCount != 1 || requestCount == 0 {
		t.Fatalf("got %d metadata events and %d request events", metadataCount, requestCount)
	}
	if strings.Contains(server.output.String(), testAPIKey) {
		t.Fatal("process logs exposed the API key")
	}
}

func expectHTTP(t *testing.T, client *http.Client, baseURL, method, path, body string, want int, wantBody string) {
	t.Helper()
	response, gotBody := sendHTTP(t, client, baseURL, method, path, body, "Bearer "+testAPIKey)
	if response.StatusCode != want || gotBody != wantBody {
		t.Fatalf("%s %s = %d %q; want %d %q", method, path, response.StatusCode, gotBody, want, wantBody)
	}
}
