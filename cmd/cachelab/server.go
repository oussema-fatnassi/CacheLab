package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/oussema-fatnassi/CacheLab/api"
	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// serverConfig contains validated HTTP settings and the required API credential.
type serverConfig struct {
	address         string
	apiKey          string
	maxBodyBytes    int64
	shutdownTimeout time.Duration
}

// loadServerConfig requires API_KEY to use Bearer token syntax and validates
// optional settings. Defaults are port 8080, an 8 MiB body limit, and a 10s
// shutdown timeout. Configuration errors never include the API key value.
func loadServerConfig() (serverConfig, error) {
	apiKey := os.Getenv("API_KEY")
	if !validAPIKey(apiKey) {
		return serverConfig{}, fmt.Errorf("API_KEY is required and must be a nonempty Bearer token without whitespace")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return serverConfig{}, fmt.Errorf("PORT must be an integer between 1 and 65535")
	}
	maxBody, err := positiveSetting("MAX_BODY_BYTES", 8*1024*1024)
	if err != nil {
		return serverConfig{}, err
	}

	timeout := 10 * time.Second
	if value := os.Getenv("SHUTDOWN_TIMEOUT"); value != "" {
		timeout, err = time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return serverConfig{}, fmt.Errorf("SHUTDOWN_TIMEOUT must be a positive duration, for example 10s")
		}
	}
	return serverConfig{address: ":" + port, apiKey: apiKey, maxBodyBytes: int64(maxBody), shutdownTimeout: timeout}, nil
}

// routeHandlers supplies CRUD handlers; nil entries leave their routes unregistered.
type routeHandlers struct{ create, read, update, delete http.Handler }

// registerRoutes installs public endpoints and supplied CRUD handlers.
// Create and update bodies are limited before reaching their handlers.
// Authentication is applied separately by newCacheServer around the whole mux.
func registerRoutes(handlers routeHandlers, maxBodyBytes int64) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /swagger/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := fmt.Fprint(w, `<!DOCTYPE html><html><head><title>CacheLab API</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.34.1/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5.34.1/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url: '/swagger/openapi.yaml', dom_id: '#swagger-ui'});</script></body></html>`); err != nil {
			log.Printf("writing response: %v", err)
		}
	})
	mux.HandleFunc("GET /swagger/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		if _, err := w.Write(api.OpenAPI); err != nil {
			log.Printf("writing response: %v", err)
		}
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintln(w, `{"status":"ok"}`); err != nil {
			log.Printf("writing response: %v", err)
		}
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Hello, World!"); err != nil {
			log.Printf("writing response: %v", err)
		}
	})
	if handlers.create != nil {
		mux.Handle("POST /keys", limitBody(handlers.create, maxBodyBytes))
	}
	if handlers.read != nil {
		mux.Handle("GET /keys/{key}", handlers.read)
	}
	if handlers.update != nil {
		mux.Handle("PUT /keys/{key}", limitBody(handlers.update, maxBodyBytes))
	}
	if handlers.delete != nil {
		mux.Handle("DELETE /keys/{key}", handlers.delete)
	}
	return mux
}

// limitBody buffers at most limit bytes and restores the body for the handler.
// Oversized bodies return 413; other read failures return 400 without calling next.
func limitBody(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			} else {
				writeError(w, http.StatusBadRequest, "invalid request body")
			}
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

// recoverPanics turns a panic in a handler into a generic 500 response,
// so later requests can still be served.
func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				// The panic value and the raw URL may contain keys, values or secrets:
				// only the request ID is logged, to find the matching request event.
				logger.Error("handler panic recovered", "request_id", w.Header().Get("X-Request-ID"))
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CacheServer serves a shared cache service over HTTP and supports graceful shutdown.
type CacheServer struct {
	service    *cache.CacheService
	httpServer *http.Server
}

// newCacheServer serves the routes with the default logger.
func newCacheServer(config serverConfig, service *cache.CacheService) *CacheServer {
	return &CacheServer{service: service, httpServer: &http.Server{Addr: config.address, Handler: serverHandler(config, service, slog.Default())}}
}

// serverHandler wraps routing with request logging and metrics, panic recovery and authentication.
// Logging comes first so refused and recovered requests are logged and counted with their real status.
// Authentication runs before route matching or body reads, so unlisted endpoints
// and future routes require credentials unless explicitly made public.
func serverHandler(config serverConfig, service *cache.CacheService, logger *slog.Logger) http.Handler {
	routes := registerRoutes(routeHandlers{create: handleCreate(service), read: handleRead(service), update: handleUpdate(service), delete: handleDelete(service)}, config.maxBodyBytes)
	metrics := newServerMetrics()
	// /metrics is not in the public list of authenticate, so it requires the API key.
	routes.Handle("GET /metrics", metrics)
	return logRequests(logger, metrics, routes, recoverPanics(logger, authenticate(jsonRoutingErrors(routes), config.apiKey)))
}

// Serve accepts requests on listener until shutdown or a serving error.
// A normal shutdown returns nil instead of http.ErrServerClosed.
func (s *CacheServer) Serve(listener net.Listener) error {
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// ListenAndServe binds the configured TCP address and delegates to Serve.
// Bind failures include the address to help diagnose startup errors.
func (s *CacheServer) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.httpServer.Addr, err)
	}
	return s.Serve(listener)
}

// Shutdown stops accepting connections and waits for active requests to finish.
// If ctx expires first, it returns the context error.
func (s *CacheServer) Shutdown(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
