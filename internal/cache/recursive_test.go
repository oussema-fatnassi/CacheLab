package cache

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func recursiveWithConfig(t *testing.T, config RecursiveConfig) *RecursiveTable {
	t.Helper()
	table, err := NewRecursiveTableWithConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestRecursiveConfiguration(t *testing.T) {
	for _, config := range []RecursiveConfig{
		{BranchCount: -1}, {BranchCount: 1}, {BranchCount: 3},
		{BranchCount: 512}, {MaxDepth: -1}, {MaxDepth: 17},
		{BranchCount: 256, MaxDepth: 9},
	} {
		if table, err := NewRecursiveTableWithConfig(config); err == nil || table != nil {
			t.Fatalf("config %+v: table=%v err=%v; want nil table and error", config, table, err)
		}
	}
	for _, branches := range []int{0, 2, 4, 8, 16, 32, 64, 128, 256} {
		t.Run(fmt.Sprint(branches), func(t *testing.T) {
			newTable := func() Table {
				return recursiveWithConfig(t, RecursiveConfig{BranchCount: branches})
			}
			runTableConformance(t, newTable)
		})
	}
}

func TestRecursiveSubdivisions(t *testing.T) {
	for _, test := range []struct {
		name            string
		branches, limit int
		second          uint64
		depth           int
	}{
		{"first", 16, 0, 1, 1},
		{"prefix", 16, 0, 0x100, 3},
		{"last", 16, 0, 1 << 60, 16},
		{"binary", 2, 0, 1 << 63, 64},
		{"partial-last-group", 8, 0, 1 << 63, 22},
		{"byte-groups", 256, 0, 1 << 56, 8},
		{"limited", 16, 2, 1 << 60, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			hashes := map[string]uint64{"a": 0, "b": test.second, "neighbor": 1}
			table := recursiveWithConfig(t, RecursiveConfig{
				BranchCount: test.branches, MaxDepth: test.limit,
				Hash: func(key string) uint64 { return hashes[key] },
			})
			for _, key := range []string{"a", "b", "neighbor"} {
				if err := table.Create(key, key); err != nil {
					t.Fatal(err)
				}
			}
			if got := table.Stats(); got.MaxDepth != test.depth || got.Entries != 3 {
				t.Fatalf("Stats = %+v; want depth %d and 3 entries", got, test.depth)
			}
			for _, key := range []string{"a", "b", "neighbor"} {
				requireTableRead(t, table, key, key, true)
			}
			if err := table.Update("b", "changed"); err != nil {
				t.Fatal(err)
			}
			if !table.Delete("a") {
				t.Fatal("delete a failed")
			}
			requireTableRead(t, table, "b", "changed", true)
			requireTableRead(t, table, "neighbor", "neighbor", true)
			if err := table.Create("a", "again"); err != nil {
				t.Fatal(err)
			}
			requireTableRead(t, table, "a", "again", true)
		})
	}
}

func TestRecursiveFullCollisionAndLaterSplit(t *testing.T) {
	for _, target := range []string{"a", "b", "c"} {
		t.Run(target, func(t *testing.T) {
			table := recursiveWithConfig(t, RecursiveConfig{Hash: func(key string) uint64 {
				if key == "other" {
					return 0x100
				}
				return 0
			}})
			for _, key := range []string{"a", "b", "c"} {
				if err := table.Create(key, key); err != nil {
					t.Fatal(err)
				}
			}
			if got := table.Stats(); got.Branches != 0 || got.MaxDepth != 0 {
				t.Fatalf("full collision must stay in one leaf: %+v", got)
			}
			if err := table.Create(target, "duplicate"); !errors.Is(err, ErrKeyExists) {
				t.Fatal(err)
			}
			if err := table.Create("other", "other"); err != nil {
				t.Fatal(err)
			}
			if err := table.Update(target, "changed"); err != nil {
				t.Fatal(err)
			}
			requireTableRead(t, table, target, "changed", true)
			if !table.Delete(target) {
				t.Fatal("delete failed")
			}
			for _, key := range []string{"a", "b", "c", "other"} {
				if key != target {
					requireTableRead(t, table, key, key, true)
				}
			}
		})
	}
}

func TestRecursivePrunesEmptyBranches(t *testing.T) {
	table := recursiveWithConfig(t, RecursiveConfig{Hash: func(key string) uint64 {
		if key == "b" {
			return 0x100
		}
		return 0
	}})
	for _, key := range []string{"a", "b"} {
		if err := table.Create(key, key); err != nil {
			t.Fatal(err)
		}
	}
	if !table.Delete("b") {
		t.Fatal("delete b failed")
	}
	if got := table.Stats(); got.Branches != 3 || got.MaxDepth != 3 || got.Entries != 1 {
		t.Fatalf("nonempty branches should remain: %+v", got)
	}
	if !table.Delete("a") {
		t.Fatal("delete a failed")
	}
	if got := table.Stats(); got.Branches != 0 || got.MaxDepth != 0 || got.Entries != 0 || got.Subdivisions != 3 {
		t.Fatalf("empty tree with cumulative subdivisions expected: %+v", got)
	}
	if err := table.Create("b", "again"); err != nil {
		t.Fatal(err)
	}
	requireTableRead(t, table, "b", "again", true)
}

func TestRecursiveHashesOncePerOperation(t *testing.T) {
	calls := 0
	table := recursiveWithConfig(t, RecursiveConfig{Hash: func(key string) uint64 {
		calls++
		if key == "b" {
			return 1 << 60
		}
		return 0
	}})
	operations := []func(){
		func() { _ = table.Create("a", "a") },
		func() { _ = table.Create("b", "b") },
		func() { _ = table.Create("a", "duplicate") },
		func() { table.Read("b") },
		func() { _ = table.Update("b", "updated") },
		func() { table.Delete("b") },
		func() { table.Read("missing") },
		func() { _ = table.Update("missing", "value") },
		func() { table.Delete("missing") },
	}
	for i, operation := range operations {
		operation()
		if calls != i+1 {
			t.Fatalf("operation %d: hash calls = %d", i, calls)
		}
	}
}

func TestRecursiveConcurrentSubdivisionsAndStats(t *testing.T) {
	newTable := func() Table {
		return recursiveWithConfig(t, RecursiveConfig{Hash: func(key string) uint64 { return hash(key) << 32 }})
	}
	runTableConcurrentConformance(t, newTable)
	table := newTable().(*RecursiveTable)
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("%d/%d", worker, i)
				if err := table.Create(key, key); err != nil {
					t.Error(err)
				}
				if got := table.Stats(); got.MaxDepth > 16 {
					t.Errorf("depth = %d", got.MaxDepth)
				}
			}
		}()
	}
	workers.Wait()
	if got := table.Stats().Entries; got != 400 {
		t.Fatalf("entries = %d", got)
	}
}

func TestRecursiveLimitedDepthConformance(t *testing.T) {
	newTable := func() Table {
		return recursiveWithConfig(t, RecursiveConfig{
			BranchCount: 4, MaxDepth: 1,
			Hash: func(key string) uint64 { return hash(key) << 2 },
		})
	}
	runTableConformance(t, newTable)
	runTableConcurrentConformance(t, newTable)
}

func TestRecursiveServiceByteLimits(t *testing.T) {
	table := NewRecursiveTable()
	service := NewCacheService(table, 2, 4)
	if err := service.Create("é", "😀"); err != nil {
		t.Fatal(err)
	}
	if err := service.Create("€", "ok"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("three-byte key: %v", err)
	}
	if err := service.Update("é", "😀x"); !errors.Is(err, ErrValueTooLarge) {
		t.Fatalf("five-byte value: %v", err)
	}
	requireTableRead(t, table, "é", "😀", true)
	if err := service.Update("é", ""); err != nil {
		t.Fatal(err)
	}
	requireTableRead(t, table, "é", "", true)
	// A second service can choose different limits without changing the engine.
	other := NewRecursiveTable()
	if err := NewCacheService(other, 3, 5).Create("€", "😀x"); err != nil {
		t.Fatal(err)
	}
	requireTableRead(t, table, "€", "", false)
	requireTableRead(t, other, "é", "", false)
}
