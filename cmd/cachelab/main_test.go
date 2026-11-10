package main

import "testing"

func TestAddressUsesConfiguredPort(t *testing.T) {
	t.Setenv("PORT", "8123")
	if got := address(); got != ":8123" {
		t.Fatalf("address() = %q, want :8123", got)
	}
}

func TestAddressDefaultsPort(t *testing.T) {
	t.Setenv("PORT", "")
	if got := address(); got != ":8080" {
		t.Fatalf("address() = %q, want :8080", got)
	}
}
