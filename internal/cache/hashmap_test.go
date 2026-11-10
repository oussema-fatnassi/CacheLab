package cache

import (
	"errors"
	"testing"
)

func TestHashTableCRUD(t *testing.T) {
	table := NewHashTable(16)

	// Create an entry.
	if err := table.Create("name", "Test"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read the entry and verify its value.
	value, found := table.Read("name")
	if !found || value != "Test" {
		t.Fatalf("Read = (%q, %t); want (%q, true)",
			value, found, "Test")
	}
	t.Logf("After Create: name = %q", value)

	// Update the existing entry.
	if err := table.Update("name", "Alice"); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	value, found = table.Read("name")
	if !found || value != "Alice" {
		t.Fatalf("Read after Update = (%q, %t); want (%q, true)",
			value, found, "Alice")
	}
	t.Logf("After Update: name = %q", value)

	// Delete the entry.
	if !table.Delete("name") {
		t.Fatal("Delete returned false for an existing key")
	}

	if _, found := table.Read("name"); found {
		t.Fatal("key still exists after Delete")
	}

	if table.size != 0 {
		t.Fatalf("size = %d; want 0", table.size)
	}
	t.Log("After Delete: key absent, size = 0")
}

func TestHashTableDuplicate(t *testing.T) {
	table := NewHashTable(16)

	if err := table.Create("name", "Test"); err != nil {
		t.Fatal(err)
	}

	if err := table.Create("name", "Alice"); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("got %v; want ErrKeyExists", err)
	}

	value, found := table.Read("name")
	if !found || value != "Test" || table.size != 1 {
		t.Fatal("duplicate Create changed the original entry or size")
	}
}

func TestHashTableMissingKey(t *testing.T) {
	table := NewHashTable(16)

	if _, found := table.Read("missing"); found {
		t.Fatal("Read returned true for a missing key")
	}

	if err := table.Update("missing", "value"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v; want ErrNotFound", err)
	}

	if table.Delete("missing") {
		t.Fatal("Delete returned true for a missing key")
	}

	if table.size != 0 {
		t.Fatalf("size = %d; want 0", table.size)
	}
}

func TestHashTableCollision(t *testing.T) {
	// Insertion at the head produces this chain: country -> city -> name.
	entries := []struct {
		key   string
		value string
	}{
		{"name", "Test"},
		{"city", "Paris"},
		{"country", "France"},
	}

	for _, target := range entries {
		t.Run(target.key, func(t *testing.T) {
			// One bucket forces collisions for every key.
			table := NewHashTable(1)
			for _, item := range entries {
				if err := table.Create(item.key, item.value); err != nil {
					t.Fatalf("Create(%q): %v", item.key, err)
				}
			}

			// Every colliding key must retain its own value.
			for _, item := range entries {
				value, found := table.Read(item.key)
				if !found || value != item.value {
					t.Fatalf("Read(%q) = (%q, %t); want (%q, true)",
						item.key, value, found, item.value)
				}
			}

			if err := table.Create(target.key, "duplicate"); !errors.Is(err, ErrKeyExists) {
				t.Fatalf("duplicate Create: got %v; want ErrKeyExists", err)
			}
			if _, found := table.Read("missing"); found {
				t.Fatal("Read found a missing key in an occupied bucket")
			}
			if err := table.Update("missing", "value"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("missing Update: got %v; want ErrNotFound", err)
			}
			if table.Delete("missing") {
				t.Fatal("Delete found a missing key in an occupied bucket")
			}

			// Each subtest updates a different chain position.
			if err := table.Update(target.key, "updated"); err != nil {
				t.Fatal(err)
			}
			for _, item := range entries {
				want := item.value
				if item.key == target.key {
					want = "updated"
				}
				if value, found := table.Read(item.key); !found || value != want {
					t.Fatalf("Read(%q) after Update = (%q, %t); want (%q, true)",
						item.key, value, found, want)
				}
			}
			if table.size != len(entries) {
				t.Fatalf("size = %d; want %d", table.size, len(entries))
			}

			// Delete the target and verify that the remaining chain survives.
			if !table.Delete(target.key) {
				t.Fatalf("Delete(%q) returned false", target.key)
			}
			if _, found := table.Read(target.key); found {
				t.Fatal("deleted key is still readable")
			}
			if table.Delete(target.key) {
				t.Fatal("repeated Delete returned true")
			}
			if table.size != len(entries)-1 {
				t.Fatalf("size = %d; want %d", table.size, len(entries)-1)
			}
			for _, item := range entries {
				if item.key == target.key {
					continue
				}
				if value, found := table.Read(item.key); !found || value != item.value {
					t.Fatalf("Read(%q) after Delete = (%q, %t); want (%q, true)",
						item.key, value, found, item.value)
				}
			}
			t.Logf("Colliding keys coexist; updating and deleting %q preserves the others", target.key)
		})
	}
}
