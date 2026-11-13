package cache

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TODO: Reuse these concurrent scenarios for future engines through the shared
// constructor registry, including forced hashes only when supported
func runTableConcurrentConformance(t *testing.T, newTable func() Table) {
	t.Helper()
	t.Run("ConcurrentDuplicateCreate", func(t *testing.T) {
		table := newTable()
		const count = 8
		type result struct {
			value string
			err   error
		}
		start := make(chan struct{})
		results := make(chan result, count)
		for i := 0; i < count; i++ {
			go func() {
				value := fmt.Sprintf("winner:%d", i)
				<-start
				results <- result{value, table.Create("shared", value)}
			}()
		}
		close(start)
		successes, duplicates, winner := 0, 0, ""
		for i := 0; i < count; i++ {
			result := <-results
			switch {
			case result.err == nil:
				successes++
				winner = result.value
			case errors.Is(result.err, ErrKeyExists):
				duplicates++
			default:
				t.Errorf("Create = %v; want nil or ErrKeyExists", result.err)
			}
		}
		if successes != 1 || duplicates != count-1 {
			t.Fatalf("successes=%d duplicates=%d; want 1, %d", successes, duplicates, count-1)
		}
		requireTableRead(t, table, "shared", winner, true)
	})

	t.Run("ConcurrentDelete", func(t *testing.T) {
		table := newTable()
		if err := table.Create("shared", "value"); err != nil {
			t.Fatal(err)
		}
		const count = 8
		start := make(chan struct{})
		results := make(chan bool, count)
		for i := 0; i < count; i++ {
			go func() {
				<-start
				results <- table.Delete("shared")
			}()
		}
		close(start)
		successes := 0
		for i := 0; i < count; i++ {
			if <-results {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("successful deletes=%d; want 1", successes)
		}
		requireTableRead(t, table, "shared", "", false)
	})

	t.Run("ConcurrentReadUpdate", func(t *testing.T) {
		table := newTable()
		first, second := strings.Repeat("first", 1000), strings.Repeat("second", 1000)
		if err := table.Create("shared", first); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var workers sync.WaitGroup
		for worker := 0; worker < 5; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				for i := 0; i < 256; i++ {
					if worker == 0 {
						value := first
						if i%2 == 0 {
							value = second
						}
						if err := table.Update("shared", value); err != nil {
							t.Errorf("Update: %v", err)
							return
						}
					} else if value, found := table.Read("shared"); !found || (value != first && value != second) {
						t.Error("Read returned a missing or incomplete value")
						return
					}
				}
			}()
		}
		close(start)
		workers.Wait()
		requireTableRead(t, table, "shared", first, true)
	})

	t.Run("ConcurrentIndependentKeys", func(t *testing.T) {
		table := newTable()
		const workerCount, keysPerWorker = 8, 16
		start := make(chan struct{})
		var workers sync.WaitGroup
		for worker := 0; worker < workerCount; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				for i := 0; i < keysPerWorker; i++ {
					key := fmt.Sprintf("worker:%d/key:%d", worker, i)
					if err := table.Create(key, "initial"); err != nil {
						t.Errorf("Create(%q): %v", key, err)
						return
					}
					if err := table.Update(key, "updated:"+key); err != nil {
						t.Errorf("Update(%q): %v", key, err)
						return
					}
					if value, found := table.Read(key); !found || value != "updated:"+key {
						t.Errorf("Read(%q) returned a missing or changed value", key)
					}
					if i%2 == 0 && !table.Delete(key) {
						t.Errorf("Delete(%q) returned false", key)
					}
				}
			}()
		}
		close(start)
		workers.Wait()
		for worker := 0; worker < workerCount; worker++ {
			for i := 0; i < keysPerWorker; i++ {
				key := fmt.Sprintf("worker:%d/key:%d", worker, i)
				if i%2 == 0 {
					requireTableRead(t, table, key, "", false)
				} else {
					requireTableRead(t, table, key, "updated:"+key, true)
				}
			}
		}
	})
}
