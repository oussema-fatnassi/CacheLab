package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

type serverConfig struct {
	address         string
	maxBodyBytes    int64
	shutdownTimeout time.Duration
}

func loadServerConfig() (serverConfig, error) {
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
	return serverConfig{address: ":" + port, maxBodyBytes: int64(maxBody), shutdownTimeout: timeout}, nil
}

type routeHandlers struct{ create, read, update, delete http.Handler }

func registerRoutes(handlers routeHandlers, maxBodyBytes int64) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
func limitBody(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "invalid request body", http.StatusBadRequest)
			}
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

type CacheServer struct {
	service    *cache.CacheService
	httpServer *http.Server
}

func newCacheServer(config serverConfig, service *cache.CacheService) *CacheServer {
	return &CacheServer{service: service, httpServer: &http.Server{Addr: config.address, Handler: registerRoutes(routeHandlers{create: handleCreate(service), read: handleRead(service)}, config.maxBodyBytes)}}
}
func (s *CacheServer) Serve(listener net.Listener) error {
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *CacheServer) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.httpServer.Addr, err)
	}
	return s.Serve(listener)
}

func (s *CacheServer) Shutdown(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
