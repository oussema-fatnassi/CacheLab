package main

import (
	"bytes"
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
// it builds the server, runs it as a separate process, stores a key, stops the
// process, starts a new one on the same port and checks that the key is gone.
// When other storage engines are added, repeat this scenario for each of them.
func TestCacheIsEmptyAfterProcessRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and starts real server processes")
	}
	binary := buildServer(t)
	port := freePort(t)
	baseURL := "http://127.0.0.1:" + port

	first := startServer(t, binary, port)
	expectStatus(t, baseURL, http.MethodPost, "/keys", `{"key": "before-restart", "value": "v"}`, http.StatusCreated)
	expectStatus(t, baseURL, http.MethodGet, "/keys/before-restart", "", http.StatusOK)
	stopServer(t, first)

	second := startServer(t, binary, port)
	defer stopServer(t, second)
	// The key stored by the first process does not survive the restart.
	expectStatus(t, baseURL, http.MethodGet, "/keys/before-restart", "", http.StatusNotFound)
	// The new process accepts new entries and the whole CRUD works.
	expectStatus(t, baseURL, http.MethodPost, "/keys", `{"key": "after-restart", "value": "v"}`, http.StatusCreated)
	expectStatus(t, baseURL, http.MethodGet, "/keys/after-restart", "", http.StatusOK)
	expectStatus(t, baseURL, http.MethodPut, "/keys/after-restart", `{"value": "new"}`, http.StatusOK)
	expectStatus(t, baseURL, http.MethodDelete, "/keys/after-restart", "", http.StatusNoContent)
	expectStatus(t, baseURL, http.MethodGet, "/keys/after-restart", "", http.StatusNotFound)
}

// buildServer compiles this package into a temporary executable.
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
func startServer(t *testing.T, binary, port string) serverProcess {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "PORT="+port, "API_KEY="+testAPIKey)
	output := &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	server := serverProcess{cmd: cmd, output: output}

	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		response, err := http.Get("http://127.0.0.1:" + port + "/health")
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
	if err := server.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// Wait reports the kill as an error; it only releases the process resources.
	_ = server.cmd.Wait()
}

// expectStatus sends an authenticated request and fails unless it gets want.
func expectStatus(t *testing.T, baseURL, method, path, body string, want int) {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testAPIKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("%s %s = %d; want %d", method, path, response.StatusCode, want)
	}
}
