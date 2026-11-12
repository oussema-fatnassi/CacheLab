package cache

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestHashTableConcurrentCreate(t *testing.T) {
	table := NewHashTable(4)
	start := make(chan struct{})
	results := make(chan error, 2)

	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- table.Create("shared", "value")
		}()
	}
	close(start)

	successes, duplicates := 0, 0
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, ErrKeyExists):
			duplicates++
		default:
			t.Errorf("unexpected Create error: %v", err)
		}
	}
	if successes != 1 || duplicates != 1 || table.size != 1 {
		t.Fatalf("successes=%d, duplicates=%d, size=%d; want 1, 1, 1",
			successes, duplicates, table.size)
	}
}

func TestHashTableConcurrentReadUpdate(t *testing.T) {
	table := NewHashTable(4)
	oldValue := strings.Repeat("old", 1000)
	newValue := strings.Repeat("new", 1000)
	if err := table.Create("shared", oldValue); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(5)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 1000; i++ {
			value := oldValue
			if i%2 == 0 {
				value = newValue
			}
			if err := table.Update("shared", value); err != nil {
				t.Errorf("Update: %v", err)
				return
			}
		}
	}()
	for i := 0; i < 4; i++ {
		go func() {
			defer workers.Done()
			<-start
			for j := 0; j < 1000; j++ {
				value, found := table.Read("shared")
				if !found || (value != oldValue && value != newValue) {
					t.Error("Read returned a missing or incomplete value")
					return
				}
			}
		}()
	}
	close(start)
	workers.Wait()
}

func TestHashTableConcurrentMixed(t *testing.T) {
	const workerCount, keysPerWorker = 8, 16
	table := NewHashTable(4)
	keys := make([]string, 0, workerCount*keysPerWorker)
	// These keys collide at every capacity reached here, up to 256 buckets.
	for i := 0; len(keys) < cap(keys); i++ {
		key := fmt.Sprintf("key:%d", i)
		if bucketIndex(key, 256) == 0 {
			keys = append(keys, key)
		}
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		owned := keys[worker*keysPerWorker : (worker+1)*keysPerWorker]
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for _, key := range owned {
				if err := table.Create(key, "initial:"+key); err != nil {
					t.Errorf("Create(%q): %v", key, err)
					return
				}
			}
			for i, key := range owned {
				if err := table.Update(key, "updated:"+key); err != nil {
					t.Errorf("Update(%q): %v", key, err)
					return
				}
				if value, found := table.Read(key); !found || value != "updated:"+key {
					t.Errorf("Read(%q) returned an unexpected value", key)
				}
				if i%2 == 0 && !table.Delete(key) {
					t.Errorf("Delete(%q) returned false", key)
				}
			}
		}()
	}
	close(start)
	workers.Wait()

	// Each worker owns distinct keys. Their histories give a deterministic
	// final result despite interleaved operations on shared chains and buckets.
	for i, key := range keys {
		value, found := table.Read(key)
		if i%2 == 0 {
			if found {
				t.Errorf("deleted key %q is still present", key)
			}
		} else if !found || value != "updated:"+key {
			t.Errorf("retained key %q is missing or changed", key)
		}
	}
	// Workers have finished, so direct inspection here is safe.
	if table.size != len(keys)/2 {
		t.Errorf("size=%d; want %d", table.size, len(keys)/2)
	}
	if len(table.buckets) <= 4 {
		t.Error("expected automatic resizing during the workload")
	}
}
