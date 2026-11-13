package cache

import (
	"errors"
	"fmt"
	"math"
	"testing"
)

func TestHashTableCRUD(t *testing.T) {
	table := NewHashTable(16)

	// Create an entry.
	if err := table.Create("name", "Test"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// read the entry and verify its value
	value, found := table.Read("name")
	if !found || value != "Test" {
		t.Fatalf("Read = (%q, %t); want (%q, true)",
			value, found, "Test")
	}
	t.Logf("After Create: name = %q", value)

	// update the existing entry
	if err := table.Update("name", "Alice"); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	value, found = table.Read("name")
	if !found || value != "Alice" {
		t.Fatalf("Read after Update = (%q, %t); want (%q, true)",
			value, found, "Alice")
	}
	t.Logf("After Update: name = %q", value)

	// delete the entry
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
	// insertion at the head produces this chain: country -> city -> name
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
			// keep one bucket for this test to exercise every chain position
			table := NewHashTableWithConfig(1, 4, 2)
			for _, item := range entries {
				if err := table.Create(item.key, item.value); err != nil {
					t.Fatalf("Create(%q): %v", item.key, err)
				}
			}

			// every colliding key must retain its own value
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

			// each subtest updates a different chain position
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

			// delete the target and verify that the remaining chain survives
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

func TestHashTableResizeThreshold(t *testing.T) {
	table := NewHashTable(4)
	for _, key := range []string{"a", "b", "c"} {
		if err := table.Create(key, key); err != nil {
			t.Fatal(err)
		}
		if len(table.buckets) != 4 {
			t.Fatal("table grew before exceeding the 0.75 threshold")
		}
	}

	// a duplicate must not change size or trigger growth at the threshold
	if err := table.Create("a", "duplicate"); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("got %v; want ErrKeyExists", err)
	}
	if table.size != 3 || len(table.buckets) != 4 {
		t.Fatal("duplicate changed size or capacity")
	}

	if err := table.Create("d", "d"); err != nil {
		t.Fatal(err)
	}
	if len(table.buckets) != 8 || table.size != 4 {
		t.Fatalf("capacity = %d, size = %d; want 8, 4", len(table.buckets), table.size)
	}
	for _, key := range []string{"a", "b", "c", "d"} {
		if value, found := table.Read(key); !found || value != key {
			t.Fatalf("key %q lost or changed after resize", key)
		}
	}
	t.Log("3 entries: 4 buckets; 4 entries: 8 buckets; all values preserved")
}

func TestHashTableResizeCollisions(t *testing.T) {
	table := NewHashTable(4)
	keys := make([]string, 0, 40)

	// These keys collide at every capacity used here: 4, 8, 16, 32 and 64
	for i := 0; len(keys) < 40; i++ {
		key := fmt.Sprintf("key:%d", i)
		if bucketIndex(key, 64) == 0 {
			keys = append(keys, key)
		}
	}

	for i, key := range keys {
		if err := table.Create(key, "value:"+key); err != nil {
			t.Fatal(err)
		}
		// verify all prior entries after every insertion and possible resize
		for _, stored := range keys[:i+1] {
			if value, found := table.Read(stored); !found || value != "value:"+stored {
				t.Fatalf("key %q lost or changed after insertion %d", stored, i+1)
			}
		}
	}
	if len(table.buckets) != 64 || table.size != len(keys) {
		t.Fatalf("capacity = %d, size = %d; want 64, %d", len(table.buckets), table.size, len(keys))
	}

	// count physical entries too, detecting duplicates or cycles after relinking.
	seen := make(map[string]bool)
	for index, entry := range table.buckets {
		for ; entry != nil; entry = entry.next {
			if seen[entry.key] {
				t.Fatalf("duplicate entry or cycle for %q", entry.key)
			}
			seen[entry.key] = true
			if bucketIndex(entry.key, len(table.buckets)) != index {
				t.Fatalf("key %q is in the wrong bucket", entry.key)
			}
		}
	}
	if len(seen) != len(keys) {
		t.Fatalf("found %d physical entries; want %d", len(seen), len(keys))
	}

	// crud must still work after several resizes
	if err := table.Update(keys[0], "updated"); err != nil {
		t.Fatal(err)
	}
	if value, found := table.Read(keys[0]); !found || value != "updated" {
		t.Fatal("Update failed after resize")
	}
	if !table.Delete(keys[1]) {
		t.Fatal("Delete failed after resize")
	}
	if _, found := table.Read(keys[1]); found {
		t.Fatal("deleted key remains after resize")
	}
	if err := table.Create(keys[1], "recreated"); err != nil {
		t.Fatal(err)
	}
	if value, found := table.Read(keys[1]); !found || value != "recreated" {
		t.Fatal("Create failed after resize")
	}
	if table.size != len(keys) {
		t.Fatalf("size = %d; want %d", table.size, len(keys))
	}
	t.Log("40 colliding keys survived growth from 4 to 64 buckets; CRUD still works")
}

func TestHashTableResizeConfig(t *testing.T) {
	table := NewHashTableWithConfig(4, 0.5, 3)
	for _, key := range []string{"a", "b"} {
		if err := table.Create(key, key); err != nil {
			t.Fatal(err)
		}
	}
	if len(table.buckets) != 4 {
		t.Fatal("table grew at the configured threshold instead of above it")
	}
	if err := table.Create("c", "c"); err != nil {
		t.Fatal(err)
	}
	if len(table.buckets) != 12 {
		t.Fatalf("capacity = %d; want 12", len(table.buckets))
	}
}

func TestHashTableInvalidResizeConfig(t *testing.T) {
	for _, config := range []struct {
		name   string
		load   float64
		growth int
	}{
		{"zero load", 0, 2},
		{"negative load", -1, 2},
		{"NaN load", math.NaN(), 2},
		{"infinite load", math.Inf(1), 2},
		{"no growth", 0.75, 1},
	} {
		t.Run(config.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected invalid configuration to panic")
				}
			}()
			NewHashTableWithConfig(4, config.load, config.growth)
		})
	}
}

// catch an ignored hash override, its omission in a CRUD path or resize,
// and overrides leaking into another table's operations
// TODO: Add equivalent verification for future custom engines, adapting their growth fixtures instead of assuming the chained engine's fourth-insert growth.
func TestHashTableUsesInstanceHash(t *testing.T) {
	calls := make(map[string]int)
	table := newChainedTableWithHash(func(key string) uint64 {
		calls[key]++
		return 7
	})
	if err := table.Create("first", "value"); err != nil {
		t.Fatal(err)
	}
	if calls["first"] == 0 {
		t.Fatal("Create did not use the instance hash")
	}
	before := calls["first"]
	requireTableRead(t, table, "first", "value", true)
	if calls["first"] <= before {
		t.Fatal("Read did not use the instance hash")
	}
	before = calls["first"]
	if err := table.Update("first", "updated"); err != nil {
		t.Fatal(err)
	}
	if calls["first"] <= before {
		t.Fatal("Update did not use the instance hash")
	}
	for _, key := range []string{"second", "third"} {
		if err := table.Create(key, key); err != nil {
			t.Fatal(err)
		}
	}
	before = calls["first"]
	if err := table.Create("fourth", "fourth"); err != nil {
		t.Fatal(err)
	}
	if calls["first"] <= before {
		t.Fatal("growth did not redistribute entries with the instance hash")
	}
	before = calls["first"]
	if !table.Delete("first") {
		t.Fatal("Delete returned false")
	}
	if calls["first"] <= before {
		t.Fatal("Delete did not use the instance hash")
	}
	before = calls["first"]
	for _, other := range []Table{
		NewHashTable(4),
		newChainedTableWithHash(func(string) uint64 { return 13 }),
	} {
		if err := other.Create("first", "independent"); err != nil {
			t.Fatal(err)
		}
		requireTableRead(t, other, "first", "independent", true)
	}
	if calls["first"] != before {
		t.Fatal("another instance used this table's hash")
	}
	requireTableRead(t, table, "second", "second", true)
}
