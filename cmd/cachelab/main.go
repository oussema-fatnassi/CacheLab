package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// application holds the cache service shared by the HTTP handlers.
type application struct{ service *cache.CacheService }

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
// Defaults are 16 buckets, 256 bytes per key, and 1 MiB per value.
func newApplication() (*application, error) {
	buckets, err := positiveSetting("CACHE_BUCKET_COUNT", 16)
	if err != nil {
		return nil, err
	}
	maxKeyBytes, err := positiveSetting("MAX_KEY_BYTES", 256)
	if err != nil {
		return nil, err
	}
	maxValueBytes, err := positiveSetting("MAX_VALUE_BYTES", 1024*1024)
	if err != nil {
		return nil, err
	}
	table := cache.NewHashTable(buckets)
	return &application{service: cache.NewCacheService(table, maxKeyBytes, maxValueBytes)}, nil
}

func main() {
	config, err := loadServerConfig()
	if err != nil {
		log.Fatal(err)
	}
	app, err := newApplication()
	if err != nil {
		log.Fatal(err)
	}
	server := newCacheServer(config, app.service)

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
