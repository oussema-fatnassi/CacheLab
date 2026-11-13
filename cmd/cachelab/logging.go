package main

import (
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder remembers the status code actually sent to the client.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		// Writing a body without WriteHeader sends 200.
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

// logRequests writes one JSON event per request with the method, the route
// pattern, the status actually returned, the duration and a request ID that is
// also sent back in X-Request-ID. It never logs the raw URL, the headers or the
// body, so keys, values and the API key stay out of the logs.
func logRequests(logger *slog.Logger, routes *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := rand.Text()
		w.Header().Set("X-Request-ID", requestID)
		// The pattern is found even when authentication refuses the request before routing.
		_, route := routes.Handler(r)

		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			// Nothing was written: net/http sends 200.
			status = http.StatusOK
		}
		logger.Info("http request",
			"method", r.Method,
			"route", route,
			"status", status,
			"duration_ms", float64(time.Since(started))/float64(time.Millisecond),
			"request_id", requestID,
		)
	})
}
