package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// application holds the cache service shared by the HTTP handlers.
type application struct {
	service *cache.CacheService
	engine  engineMetadata
}

// positiveSetting reads a positive integer from the environment, using fallback
// when the variable is unset or empty.
func positiveSetting(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

// newApplication builds an in-memory cache from environment settings.
// Defaults are chained with 16 buckets, 256 bytes per key, and 1 MiB per value.
func newApplication() (*application, error) {
	maxKeyBytes, err := positiveSetting("MAX_KEY_BYTES", 256)
	if err != nil {
		return nil, err
	}
	maxValueBytes, err := positiveSetting("MAX_VALUE_BYTES", 1024*1024)
	if err != nil {
		return nil, err
	}
	table, metadata, err := newConfiguredTable()
	if err != nil {
		return nil, err
	}
	return &application{service: cache.NewCacheService(table, maxKeyBytes, maxValueBytes), engine: metadata}, nil
}

func main() {
	// One JSON event per line on standard output, with the same settings for every
	// engine variant. log.Printf and log.Fatal also go through this JSON logger.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	// Recorded at startup so performance campaigns know the logging configuration.
	slog.Info("logging configured", "format", "json", "log_level", "info", "output", "stdout")

	config, err := loadServerConfig()
	if err != nil {
		log.Fatal(err)
	}
	app, err := newApplication()
	if err != nil {
		log.Fatal(err)
	}
	server := newCacheServer(config, app.service)
	logExecutionMetadata(slog.Default(), app.engine)

	// Ctrl+C or SIGTERM starts graceful shutdown.
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Report serving errors while also waiting for a shutdown signal.
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.ListenAndServe() }()
	log.Printf("Listening on %s", config.address)

	select {
	case err := <-serverDone:
		if err != nil {
			log.Fatal(err)
		}
		return
	case <-signalCtx.Done():
	}

	stop()
	// Give active requests the configured time to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	if err := <-serverDone; err != nil {
		log.Fatal(err)
	}
}
