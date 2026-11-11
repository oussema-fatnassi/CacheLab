package cache

import (
	"errors"
	"fmt"
	"testing"
)

type tableDouble struct {
	calls                 int
	operation, key, value string
	err                   error
	readValue             string
	found, deleted        bool
}

func (d *tableDouble) Create(key, value string) error {
	d.calls++
	d.operation, d.key, d.value = "create", key, value
	return d.err
}
func (d *tableDouble) Read(key string) (string, bool) {
	d.calls++
	d.operation, d.key = "read", key
	return d.readValue, d.found
}
func (d *tableDouble) Update(key, value string) error {
	d.calls++
	d.operation, d.key, d.value = "update", key, value
	return d.err
}
func (d *tableDouble) Delete(key string) bool {
	d.calls++
	d.operation, d.key = "delete", key
	return d.deleted
}

func TestServiceDelegatesAndReturnsResults(t *testing.T) {
	d := &tableDouble{readValue: "value", found: true, deleted: true}
	s := NewCacheService(d, 4, 5)
	if err := s.Create("key", "value"); err != nil || d.operation != "create" || d.key != "key" || d.value != "value" {
		t.Fatalf("create: err=%v, table=%+v", err, d)
	}
	if value, err := s.Read("key"); err != nil || value != "value" || d.operation != "read" || d.key != "key" {
		t.Fatalf("read: value=%q err=%v table=%+v", value, err, d)
	}
	if err := s.Update("key", "new"); err != nil || d.operation != "update" || d.key != "key" || d.value != "new" {
		t.Fatalf("update: err=%v table=%+v", err, d)
	}
	if err := s.Delete("key"); err != nil || d.operation != "delete" || d.key != "key" {
		t.Fatalf("delete: err=%v table=%+v", err, d)
	}
	if d.calls != 4 {
		t.Fatalf("calls=%d, want 4", d.calls)
	}
}

func TestServiceRejectsInvalidInputWithoutCallingTable(t *testing.T) {
	tests := []struct {
		name string
		call func(*CacheService) error
		want error
	}{
		{"empty create key", func(s *CacheService) error { return s.Create("", "v") }, ErrInvalidKey},
		{"empty read key", func(s *CacheService) error { _, err := s.Read(""); return err }, ErrInvalidKey},
		{"empty update key", func(s *CacheService) error { return s.Update("", "v") }, ErrInvalidKey},
		{"empty delete key", func(s *CacheService) error { return s.Delete("") }, ErrInvalidKey},
		{"multibyte key too long", func(s *CacheService) error { return s.Create("ééé", "v") }, ErrInvalidKey},
		{"create value too long", func(s *CacheService) error { return s.Create("key", "ééé") }, ErrValueTooLarge},
		{"update value too long", func(s *CacheService) error { return s.Update("key", "ééé") }, ErrValueTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &tableDouble{}
			s := NewCacheService(d, 4, 5)
			if err := tt.call(s); !errors.Is(err, tt.want) {
				t.Fatalf("error=%v want %v", err, tt.want)
			}
			if d.calls != 0 {
				t.Fatalf("table called %d times", d.calls)
			}
		})
	}
}

func TestServiceAcceptsBoundariesAndEmptyValue(t *testing.T) {
	d := &tableDouble{}
	s := NewCacheService(d, 4, 5)
	for _, value := range []string{"", "12345"} {
		if err := s.Create("éé", value); err != nil {
			t.Fatalf("value %q: %v", value, err)
		}
	}
	if d.calls != 2 {
		t.Fatalf("calls=%d, want 2", d.calls)
	}
}

func TestServicePropagatesErrorsAndMissingKeys(t *testing.T) {
	d := &tableDouble{err: fmt.Errorf("table: %w", ErrKeyExists)}
	s := NewCacheService(d, 4, 5)
	if err := s.Create("key", "v"); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("create: %v", err)
	}
	d.err = fmt.Errorf("table: %w", ErrNotFound)
	if err := s.Update("key", "v"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update: %v", err)
	}
	if _, err := s.Read("key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read: %v", err)
	}
	if err := s.Delete("key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: %v", err)
	}
}

func TestServiceWithHashTable(t *testing.T) {
	s := NewCacheService(NewHashTable(2), 20, 20)
	if err := s.Create("key", "value"); err != nil {
		t.Fatal(err)
	}
	if err := s.Create("key", "other"); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("duplicate create: %v", err)
	}
	if err := s.Update("key", "new"); err != nil {
		t.Fatal(err)
	}
	if value, err := s.Read("key"); err != nil || value != "new" {
		t.Fatalf("read: value=%q err=%v", value, err)
	}
	if err := s.Delete("key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read("key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read deleted key: %v", err)
	}
	if err := s.Update("key", "new"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update deleted key: %v", err)
	}
	if err := s.Delete("key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing key: %v", err)
	}
}
