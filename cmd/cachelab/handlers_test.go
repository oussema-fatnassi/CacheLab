package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// failingTable simulates a storage where every write fails.
type failingTable struct{}

func (failingTable) Create(key, value string) error { return errors.New("storage failure") }
func (failingTable) Read(key string) (string, bool) { return "", false }
func (failingTable) Update(key, value string) error { return errors.New("storage failure") }
func (failingTable) Delete(key string) bool         { return false }

// postKeys sends body to handleCreate and returns the recorded response.
func postKeys(service *cache.CacheService, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/keys", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handleCreate(service).ServeHTTP(recorder, request)
	return recorder
}

func TestHandleCreateStoresNewKey(t *testing.T) {
	service := cache.NewCacheService(cache.NewHashTable(16), 20, 20)

	recorder := postKeys(service, `{"key": "name", "value": "Test"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d; want %d", recorder.Code, http.StatusCreated)
	}

	if value, err := service.Read("name"); err != nil || value != "Test" {
		t.Fatalf("Read = (%q, %v); want (%q, nil)", value, err, "Test")
	}
}

func TestHandleCreateRejectsExistingKey(t *testing.T) {
	service := cache.NewCacheService(cache.NewHashTable(16), 20, 20)
	if err := service.Create("name", "Test"); err != nil {
		t.Fatal(err)
	}

	recorder := postKeys(service, `{"key": "name", "value": "Alice"}`)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d; want %d", recorder.Code, http.StatusConflict)
	}

	// The original value must not be overwritten.
	if value, err := service.Read("name"); err != nil || value != "Test" {
		t.Fatalf("Read = (%q, %v); want (%q, nil)", value, err, "Test")
	}
}

func TestHandleCreateNeverReturnsCreatedOnStorageError(t *testing.T) {
	service := cache.NewCacheService(failingTable{}, 20, 20)

	recorder := postKeys(service, `{"key": "name", "value": "Test"}`)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestHandleCreateRejectsInvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"key": "name"`},
		{"empty key", `{"key": "", "value": "Test"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := cache.NewCacheService(cache.NewHashTable(16), 20, 20)

			recorder := postKeys(service, tt.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d; want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}
