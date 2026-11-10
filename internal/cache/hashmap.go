package cache

import "errors"

var (
	ErrKeyExists = errors.New("key already exists")
	ErrNotFound  = errors.New("key not found")
)

// CacheEntry stores a key/value pair.
// next links entries that share the same bucket.
type CacheEntry struct {
	key   string
	value string
	next  *CacheEntry
}

// HashTable stores entries using separate chaining for collisions.
type HashTable struct {
	buckets []*CacheEntry
	size    int
}

// NewHashTable creates an empty table with a positive bucket count.
func NewHashTable(bucketCount int) *HashTable {
	if bucketCount <= 0 {
		panic("bucketCount must be greater than 0")
	}

	return &HashTable{
		buckets: make([]*CacheEntry, bucketCount),
	}
}

// Create inserts a new entry.
// It returns ErrKeyExists if the key already exists.
func (table *HashTable) Create(key, value string) error {
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
	return nil
}

// Read returns the value and whether the key exists.
// CacheService translates false into ErrNotFound.
func (table *HashTable) Read(key string) (string, bool) {
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
