package cache

import "hash/fnv"

// hash returns the 64-bit FNV-1a hash of the key.
// The same key always produces the same hash value.
func hash(key string) uint64 {
	hasher := fnv.New64a()
	// FNV's Write method never returns an error.
	_, _ = hasher.Write([]byte(key))
	return hasher.Sum64()
}

// bucketIndex maps a key to an index between 0 and bucketCount-1.
// Different keys may map to the same bucket; the table handles these collisions.
func bucketIndex(key string, bucketCount int) int {
	if bucketCount <= 0 {
		panic("bucketCount must be greater than 0")
	}
	return int(hash(key) % uint64(bucketCount))
}
