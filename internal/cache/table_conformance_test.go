package cache

import (
	"errors"
	"testing"
)

// register real engines here. Table is their interface :
// newTable below calls NewHashTable(4), whose methods live in hashmap.go
// Service spies do not belong in this registry
// TODO: Register recursive and Go-map constructors here once implemented; each constructor must return a fresh instance for the shared suite.
var tableConstructors = []struct {
	name        string
	newTable    func() Table
	newWithHash func(func(string) uint64) Table
}{
	{
		name:        "chained",
		newTable:    func() Table { return NewHashTable(4) },
		newWithHash: newChainedTableWithHash,
	},
}

// set the private hash before using the table. The public constructors
// still use FNV-1a; this override stays in the test fixture
func newChainedTableWithHash(hashKey func(string) uint64) Table {
	table := NewHashTable(4)
	table.hashKey = hashKey
	return table
}

// TODO: Supply newWithHash for future custom engines; leave it nil for Go map, whose runtime-controlled hash cannot participate in forced-hash fixtures.
func TestTableForcedHashConformance(t *testing.T) {
	for _, engine := range tableConstructors {
		if engine.newWithHash == nil {
			continue
		}
		t.Run(engine.name, func(t *testing.T) {
			// force every key to hash to 7; HashTable still compares the keys
			newTable := func() Table {
				return engine.newWithHash(func(string) uint64 { return 7 })
			}
			runTableConformance(t, newTable)
			runTableConcurrentConformance(t, newTable)
		})
	}
}

// pass the real constructor to the shared CRUD and concurrency checks
func TestTableConformance(t *testing.T) {
	for _, engine := range tableConstructors {
		t.Run(engine.name, func(t *testing.T) {
			runTableConformance(t, engine.newTable)
			runTableConcurrentConformance(t, engine.newTable)
		})
	}
}

func requireTableRead(t *testing.T, table Table, key, want string, wantFound bool) {
	t.Helper()
	if value, found := table.Read(key); value != want || found != wantFound {
		t.Fatalf("Read(%q) = (%q, %t); want (%q, %t)", key, value, found, want, wantFound)
	}
}

// TODO: Reuse this black-box suite for each future engine through the registry; keep engine-specific layout and exact growth assertions in separate tests.
// Each newTable() gives me a fresh HashTable today; Table calls its methods
// in hashmap.go. bypass CacheService and HTTP, so validation is not tested
func runTableConformance(t *testing.T, newTable func() Table) {
	t.Helper()
	// check that Create inserts the entry and Read finds its value.
	t.Run("CreateThenRead", func(t *testing.T) {
		table := newTable()
		if err := table.Create("key", "initial"); err != nil {
			t.Fatal(err)
		}
		requireTableRead(t, table, "key", "initial", true)
	})

	// check that Create's duplicate guard keeps the first value
	t.Run("DuplicatePreservesValue", func(t *testing.T) {
		table := newTable()
		if err := table.Create("key", "original"); err != nil {
			t.Fatal(err)
		}
		if err := table.Create("key", "replacement"); !errors.Is(err, ErrKeyExists) {
			t.Fatalf("duplicate Create = %v; want ErrKeyExists", err)
		}
		requireTableRead(t, table, "key", "original", true)
	})

	// replace the existing value, then check that an empty value still exists
	t.Run("UpdateIncludingEmptyValue", func(t *testing.T) {
		table := newTable()
		if err := table.Create("key", "initial"); err != nil {
			t.Fatal(err)
		}
		if err := table.Update("key", "updated"); err != nil {
			t.Fatal(err)
		}
		requireTableRead(t, table, "key", "updated", true)
		if err := table.Update("key", ""); err != nil {
			t.Fatal(err)
		}
		requireTableRead(t, table, "key", "", true)
	})

	// expect the missing-key returns without creating or changing an entry
	t.Run("MissingOperationsPreserveEntries", func(t *testing.T) {
		table := newTable()
		if err := table.Create("present", "original"); err != nil {
			t.Fatal(err)
		}
		requireTableRead(t, table, "missing", "", false)
		if err := table.Update("missing", "replacement"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing Update = %v; want ErrNotFound", err)
		}
		requireTableRead(t, table, "missing", "", false)
		if table.Delete("missing") {
			t.Fatal("Delete returned true for a missing key")
		}
		requireTableRead(t, table, "missing", "", false)
		requireTableRead(t, table, "present", "original", true)
	})

	// check that Delete unlinks only this key, then Create can insert it again
	t.Run("DeletePreservesOtherAndRecreates", func(t *testing.T) {
		table := newTable()
		if err := table.Create("key", "initial"); err != nil {
			t.Fatal(err)
		}
		if err := table.Create("other", "preserved"); err != nil {
			t.Fatal(err)
		}
		if !table.Delete("key") {
			t.Fatal("Delete returned false for an existing key")
		}
		requireTableRead(t, table, "key", "", false)
		requireTableRead(t, table, "other", "preserved", true)
		if table.Delete("key") {
			t.Fatal("Delete returned true for an already deleted key")
		}
		if err := table.Create("key", "recreated"); err != nil {
			t.Fatal(err)
		}
		requireTableRead(t, table, "key", "recreated", true)
		requireTableRead(t, table, "other", "preserved", true)
	})
}
