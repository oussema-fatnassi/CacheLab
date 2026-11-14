package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// TestNewApplicationBuildsConfiguredService checks that environment byte limits
// reach the service while the configured table remains usable for CRUD operations.
func TestNewApplicationBuildsConfiguredService(t *testing.T) {
	for _, engine := range []string{"chained", "recursive", "go-map"} {
		t.Run(engine, func(t *testing.T) {
			t.Setenv("CACHE_ENGINE", engine)
			t.Setenv("CACHE_BUCKET_COUNT", "2")
			t.Setenv("MAX_KEY_BYTES", "4")
			t.Setenv("MAX_VALUE_BYTES", "5")
			app, err := newApplication()
			if err != nil {
				t.Fatal(err)
			}
			if err := app.service.Create("key", "value"); err != nil {
				t.Fatal(err)
			}
			if got, err := app.service.Read("key"); err != nil || got != "value" {
				t.Fatalf("Read = %q, %v", got, err)
			}
			if err := app.service.Create("longer", "v"); !errors.Is(err, cache.ErrInvalidKey) {
				t.Fatalf("key limit: %v", err)
			}
			if err := app.service.Create("key2", "longer"); !errors.Is(err, cache.ErrValueTooLarge) {
				t.Fatalf("value limit: %v", err)
			}
			if err := app.service.Create("éé", "12345"); err != nil {
				t.Fatalf("exact byte limits: %v", err)
			}
		})
	}
}

func TestNewApplicationRejectsUnknownEngine(t *testing.T) {
	for _, engine := range []string{"unknown-sensitive-value", "Go-map", " chained", " "} {
		t.Run(engine, func(t *testing.T) {
			t.Setenv("CACHE_ENGINE", engine)
			t.Setenv("CACHE_BUCKET_COUNT", "2")
			t.Setenv("MAX_KEY_BYTES", "4")
			t.Setenv("MAX_VALUE_BYTES", "5")
			app, err := newApplication()
			if err == nil || app != nil {
				t.Fatal("unknown engine must prevent application startup")
			}
			if strings.Contains(err.Error(), "\""+engine+"\"") || strings.Contains(err.Error(), "unknown-sensitive-value") {
				t.Fatal("configuration error exposed the invalid value")
			}
		})
	}
}

// TestNewApplicationRejectsInvalidConfiguration checks that invalid settings prevent startup.
func TestNewApplicationRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("CACHE_ENGINE", "chained")
	t.Setenv("CACHE_BUCKET_COUNT", "2")
	t.Setenv("MAX_KEY_BYTES", "4")
	t.Setenv("MAX_VALUE_BYTES", "5")
	invalidSettings := []struct{ name, value string }{
		{"CACHE_BUCKET_COUNT", "0"},
		{"MAX_KEY_BYTES", "-1"},
		{"MAX_VALUE_BYTES", "invalid"},
	}
	for _, setting := range invalidSettings {
		t.Run(setting.name, func(t *testing.T) {
			t.Setenv(setting.name, setting.value)
			if _, err := newApplication(); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestAddressUsesConfiguredPort(t *testing.T) {
	t.Setenv("API_KEY", testAPIKey)
	t.Setenv("PORT", "8123")
	config, err := loadServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := config.address; got != ":8123" {
		t.Fatalf("address = %q, want :8123", got)
	}
}

func TestAddressDefaultsPort(t *testing.T) {
	t.Setenv("API_KEY", testAPIKey)
	t.Setenv("PORT", "")
	config, err := loadServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := config.address; got != ":8080" {
		t.Fatalf("address = %q, want :8080", got)
	}
}
