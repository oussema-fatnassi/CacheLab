package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

type application struct {
	service *cache.CacheService
}

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

func address() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}

func main() {
	app, err := newApplication()
	if err != nil {
		log.Fatal(err)
	}
	_ = app // The CRUD handlers will use the service.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "Hello, World!"); err != nil {
			log.Printf("writing response: %v", err)
		}
	})

	log.Printf("Listening on %s", address())
	log.Fatal(http.ListenAndServe(address(), mux))
}
