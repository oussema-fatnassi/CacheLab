package main

import (
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"
)

// responseStatus records the final HTTP status without changing the response body.
type responseStatus struct {
	http.ResponseWriter
	status int
}

func (w *responseStatus) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.ResponseWriter.WriteHeader(status)
	// Informational responses do not commit the final response status.
	if status >= 200 {
		w.status = status
	}
}

func (w *responseStatus) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// Unwrap allows http.ResponseController to access the underlying writer.
func (w *responseStatus) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logRequests logs only agreed fields, never URLs, bodies or request headers.
// It wraps panic recovery so recovered panics are recorded with status 500.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		// Generate the ID locally rather than trusting a client-supplied value.
		requestID := rand.Text()
		w.Header().Set("X-Request-ID", requestID)
		response := &responseStatus{ResponseWriter: w}

		next.ServeHTTP(response, r)

		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.Info("http_request",
			"method", r.Method,
			"route", r.Pattern,
			"status", status,
			"duration_ms", float64(time.Since(started))/float64(time.Millisecond),
			"request_id", requestID,
		)
	})
}
