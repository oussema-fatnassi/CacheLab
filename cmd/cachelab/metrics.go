package main

import (
	"cmp"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// latencyBuckets are the upper bounds of the latency histogram, in seconds.
var latencyBuckets = []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

// requestKey groups requests by route pattern and status code, never by cache key.
type requestKey struct {
	route  string
	status int
}

// serverMetrics counts requests in memory. Its own small mutex protects it, so
// recording a request never takes the HashTable lock nor walks the cache entries.
type serverMetrics struct {
	mu       sync.Mutex
	requests map[requestKey]uint64
	hits     uint64
	misses   uint64
	errors   uint64
	buckets  []uint64 // observations per histogram bucket; the last one is +Inf
	sum      float64  // total duration, in seconds
	count    uint64
}

func newServerMetrics() *serverMetrics {
	return &serverMetrics{
		requests: make(map[requestKey]uint64),
		buckets:  make([]uint64, len(latencyBuckets)+1),
	}
}

// observe records one finished request.
func (m *serverMetrics) observe(route string, status int, elapsed time.Duration) {
	seconds := elapsed.Seconds()
	// Index of the first bound greater than or equal to the duration, or +Inf.
	bucket, _ := slices.BinarySearch(latencyBuckets, seconds)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[requestKey{route: route, status: status}]++
	if route == "GET /keys/{key}" {
		// A read is a hit when the key exists and a miss when it does not.
		// An intentional miss is a normal answer, not an internal error.
		switch status {
		case http.StatusOK:
			m.hits++
		case http.StatusNotFound:
			m.misses++
		}
	}
	if status >= http.StatusInternalServerError {
		m.errors++
	}
	m.buckets[bucket]++
	m.sum += seconds
	m.count++
}

// ServeHTTP writes the metrics in the Prometheus text format.
func (m *serverMetrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Reading the memory statistics briefly pauses the program, so it happens
	// only when /metrics is requested, never during cache requests.
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)

	var out strings.Builder
	m.mu.Lock()
	writeRequests(&out, m.requests)
	writeCounter(&out, "cachelab_cache_hits_total", "Reads (GET /keys/{key}) that found the key.", m.hits)
	writeCounter(&out, "cachelab_cache_misses_total", "Reads (GET /keys/{key}) of a missing key (404). A miss is not an error.", m.misses)
	writeCounter(&out, "cachelab_http_errors_total", "Internal errors: responses with a 5xx status.", m.errors)
	writeHistogram(&out, m.buckets, m.sum, m.count)
	m.mu.Unlock()

	writeGauge(&out, "cachelab_go_memory_sys_bytes", "Memory obtained from the OS by the Go runtime for the whole process (heap, stacks, runtime data), in bytes.", memory.Sys)
	writeGauge(&out, "cachelab_go_heap_alloc_bytes", "Heap memory of allocated objects in the whole process, cache entries included, in bytes.", memory.HeapAlloc)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if _, err := fmt.Fprint(w, out.String()); err != nil {
		log.Printf("writing response: %v", err)
	}
}

// writeRequests writes the request counter, sorted by route then status.
func writeRequests(out *strings.Builder, requests map[requestKey]uint64) {
	keys := make([]requestKey, 0, len(requests))
	for key := range requests {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b requestKey) int {
		return cmp.Or(cmp.Compare(a.route, b.route), cmp.Compare(a.status, b.status))
	})

	fmt.Fprintln(out, "# HELP cachelab_http_requests_total HTTP requests by route pattern and status code.")
	fmt.Fprintln(out, "# TYPE cachelab_http_requests_total counter")
	for _, key := range keys {
		// %q quotes the route, so a label can never break the line format.
		fmt.Fprintf(out, "cachelab_http_requests_total{route=%q,status=\"%d\"} %d\n", key.route, key.status, requests[key])
	}
}

func writeCounter(out *strings.Builder, name, help string, value uint64) {
	fmt.Fprintf(out, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, value)
}

func writeGauge(out *strings.Builder, name, help string, value uint64) {
	fmt.Fprintf(out, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
}

// writeHistogram writes cumulative buckets: each one counts the requests
// that lasted at most its bound.
func writeHistogram(out *strings.Builder, buckets []uint64, sum float64, count uint64) {
	const name = "cachelab_http_request_duration_seconds"
	fmt.Fprintf(out, "# HELP %s Server-side request duration, from the first middleware to the end of the handler, in seconds.\n", name)
	fmt.Fprintf(out, "# TYPE %s histogram\n", name)
	var cumulative uint64
	for i, bound := range latencyBuckets {
		cumulative += buckets[i]
		fmt.Fprintf(out, "%s_bucket{le=\"%s\"} %d\n", name, strconv.FormatFloat(bound, 'g', -1, 64), cumulative)
	}
	cumulative += buckets[len(latencyBuckets)]
	fmt.Fprintf(out, "%s_bucket{le=\"+Inf\"} %d\n", name, cumulative)
	fmt.Fprintf(out, "%s_sum %s\n", name, strconv.FormatFloat(sum, 'g', -1, 64))
	fmt.Fprintf(out, "%s_count %d\n", name, count)
}
