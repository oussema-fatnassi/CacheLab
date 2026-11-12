package cache

import (
	"math"
	"sync"
)

// CacheEntry stores a key/value pair.
// next links entries that share the same bucket.
type CacheEntry struct {
	key   string
	value string
	next  *CacheEntry
}

// HashTable stores entries using separate chaining for collisions.
// CRUD operations share one mutex. Do not copy a HashTable after first use.
type HashTable struct {
	mu            sync.Mutex
	buckets       []*CacheEntry
	size          int
	maxLoadFactor float64
	growthFactor  int
}

// NewHashTable uses a load threshold of 0.75 and doubles capacity when exceeded.
func NewHashTable(bucketCount int) *HashTable {
	return NewHashTableWithConfig(bucketCount, 0.75, 2)
}

// NewHashTableWithConfig creates a table with configurable growth settings.
func NewHashTableWithConfig(bucketCount int, maxLoadFactor float64, growthFactor int) *HashTable {
	if bucketCount <= 0 {
		panic("bucketCount must be greater than 0")
	}
	if !(maxLoadFactor > 0) || math.IsInf(maxLoadFactor, 0) {
		panic("maxLoadFactor must be finite and greater than 0")
	}
	if growthFactor < 2 {
		panic("growthFactor must be at least 2")
	}

	return &HashTable{
		buckets:       make([]*CacheEntry, bucketCount),
		maxLoadFactor: maxLoadFactor,
		growthFactor:  growthFactor,
	}
}

// Create inserts a new entry.
// It returns ErrKeyExists if the key already exists.
func (table *HashTable) Create(key, value string) error {
	table.mu.Lock()
	defer table.mu.Unlock()

	index := bucketIndex(key, len(table.buckets))

	for entry := table.buckets[index]; entry != nil; entry = entry.next {
		if entry.key == key {
			return ErrKeyExists
		}
	}

	table.buckets[index] = &CacheEntry{
		key:   key,
		value: value,
		next:  table.buckets[index],
	}

	table.size++
	if float64(table.size)/float64(len(table.buckets)) > table.maxLoadFactor {
		table.resize()
	}
	return nil
}

// resize redistributes existing entries without changing their values or size.
// The caller must hold table.mu; this helper does not lock again.
func (table *HashTable) resize() {
	buckets := make([]*CacheEntry, len(table.buckets)*table.growthFactor)

	for _, entry := range table.buckets {
		for entry != nil {
			// Save the old link before moving this entry into its new chain.
			next := entry.next
			index := bucketIndex(entry.key, len(buckets))
			entry.next = buckets[index]
			buckets[index] = entry
			entry = next
		}
	}

	table.buckets = buckets
}

// Read returns the value and whether the key exists.
// CacheService translates false into ErrNotFound.
func (table *HashTable) Read(key string) (string, bool) {
	table.mu.Lock()
	defer table.mu.Unlock()

	index := bucketIndex(key, len(table.buckets))

	for entry := table.buckets[index]; entry != nil; entry = entry.next {
		if entry.key == key {
			return entry.value, true
		}
	}

	return "", false
}

// Update replaces an existing entry's value.
// It returns ErrNotFound if the key does not exist.
func (table *HashTable) Update(key, value string) error {
	table.mu.Lock()
	defer table.mu.Unlock()

	index := bucketIndex(key, len(table.buckets))

	for entry := table.buckets[index]; entry != nil; entry = entry.next {
		if entry.key == key {
			entry.value = value
			return nil
		}
	}

	return ErrNotFound
}

// Delete removes an entry and reports whether it existed.
// CacheService translates false into ErrNotFound.
func (table *HashTable) Delete(key string) bool {
	table.mu.Lock()
	defer table.mu.Unlock()

	index := bucketIndex(key, len(table.buckets))

	var previous *CacheEntry
	for entry := table.buckets[index]; entry != nil; entry = entry.next {
		if entry.key == key {
			if previous == nil {
				// Removing the first entry advances the bucket pointer.
				table.buckets[index] = entry.next
			} else {
				// Bypass this entry without losing the rest of the chain.
				previous.next = entry.next
			}
			table.size--
			return true
		}
		previous = entry
	}

	return false
}
