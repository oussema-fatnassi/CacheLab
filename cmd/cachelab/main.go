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

type application struct{ service *cache.CacheService }

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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	config, err := loadServerConfig()
	if err != nil {
		log.Fatal(err)
	}
	app, err := newApplication()
	if err != nil {
		log.Fatal(err)
	}
	server := newCacheServer(config, app.service)

	// Le contexte est annulé à la réception de Ctrl+C ou de SIGTERM.
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// L'écoute s'exécute en parallèle pour pouvoir attendre aussi un signal.
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
	// Laisser les requêtes en cours se terminer dans le délai configuré.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	if err := <-serverDone; err != nil {
		log.Fatal(err)
	}
}
