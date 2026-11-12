package main

import (
	"errors"
	"testing"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// TestNewApplicationBuildsConfiguredService verifies that configuration reaches the service and its table.
func TestNewApplicationBuildsConfiguredService(t *testing.T) {
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
}

// TestNewApplicationRejectsInvalidConfiguration checks that invalid settings prevent startup.
func TestNewApplicationRejectsInvalidConfiguration(t *testing.T) {
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
	t.Setenv("PORT", "")
	config, err := loadServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := config.address; got != ":8080" {
		t.Fatalf("address = %q, want :8080", got)
	}
}
