package cache

import "sync"

// GoMap is the reference storage engine using Go's native map and hash.
// Every operation shares one mutex. Do not copy a GoMap after first use.
type GoMap struct {
	mu      sync.Mutex
	entries map[string]string
}

// NewGoMap creates an empty, independent map without an explicit capacity hint.
func NewGoMap() *GoMap {
	return &GoMap{entries: make(map[string]string)}
}

// Create inserts a key without replacing an existing value.
func (table *GoMap) Create(key, value string) error {
	table.mu.Lock()
	defer table.mu.Unlock()
	if _, found := table.entries[key]; found {
		return ErrKeyExists
	}
	table.entries[key] = value
	return nil
}

// Read returns the exact value and whether the key exists.
func (table *GoMap) Read(key string) (string, bool) {
	table.mu.Lock()
	defer table.mu.Unlock()
	value, found := table.entries[key]
	return value, found
}

// Update replaces an existing value without inserting a missing key.
func (table *GoMap) Update(key, value string) error {
	table.mu.Lock()
	defer table.mu.Unlock()
	if _, found := table.entries[key]; !found {
		return ErrNotFound
	}
	table.entries[key] = value
	return nil
}

// Delete removes the key and reports whether it existed.
func (table *GoMap) Delete(key string) bool {
	table.mu.Lock()
	defer table.mu.Unlock()
	if _, found := table.entries[key]; !found {
		return false
	}
	delete(table.entries, key)
	return true
}
