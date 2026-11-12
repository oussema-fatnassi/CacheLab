package main

import (
	"encoding/json"
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

// newKeysMux registers the key routes the same way as main.
func newKeysMux(service *cache.CacheService) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /keys", handleCreate(service))
	mux.HandleFunc("GET /keys/{key}", handleRead(service))
	return mux
}

// send passes a request through mux and returns the recorded response.
func send(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

// decodeRead reads the JSON body of a successful GET /keys/{key}.
func decodeRead(t *testing.T, recorder *httptest.ResponseRecorder) readResponse {
	t.Helper()
	var response readResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return response
}

func TestHandleReadReturnsExactValue(t *testing.T) {
	service := cache.NewCacheService(cache.NewHashTable(16), 20, 100)
	// The value is stored directly through the service, without POST.
	value := `Café "crème" <b>&</b> 😀`
	if err := service.Create("name", value); err != nil {
		t.Fatal(err)
	}
	mux := newKeysMux(service)

	// Several reads return the same value and never modify it.
	for range 3 {
		recorder := send(mux, http.MethodGet, "/keys/name", "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d; want %d", recorder.Code, http.StatusOK)
		}
		if got := decodeRead(t, recorder); got.Key != "name" || got.Value != value {
			t.Fatalf("response = %+v; want key %q and value %q", got, "name", value)
		}
	}
	if got, err := service.Read("name"); err != nil || got != value {
		t.Fatalf("Read = (%q, %v); want (%q, nil)", got, err, value)
	}
}

func TestHandleReadErrors(t *testing.T) {
	mux := newKeysMux(cache.NewCacheService(cache.NewHashTable(16), 10, 20))

	tests := []struct {
		name string
		path string
		want int
	}{
		{"missing key", "/keys/missing", http.StatusNotFound},
		{"key too long", "/keys/averylongkey", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if recorder := send(mux, http.MethodGet, tt.path, ""); recorder.Code != tt.want {
				t.Fatalf("status = %d; want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestCreateThenReadIntegration(t *testing.T) {
	mux := newKeysMux(cache.NewCacheService(cache.NewHashTable(16), 20, 20))

	if recorder := send(mux, http.MethodPost, "/keys", `{"key": "name", "value": "Test"}`); recorder.Code != http.StatusCreated {
		t.Fatalf("POST status = %d; want %d", recorder.Code, http.StatusCreated)
	}

	recorder := send(mux, http.MethodGet, "/keys/name", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d; want %d", recorder.Code, http.StatusOK)
	}
	if got := decodeRead(t, recorder); got.Value != "Test" {
		t.Fatalf("value = %q; want %q", got.Value, "Test")
	}
}
