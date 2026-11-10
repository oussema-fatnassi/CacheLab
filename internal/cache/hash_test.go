package cache

import "testing"

func TestPrintHash(t *testing.T) {
	key := "Hello world!!" // Change this to any string.
	bucketCount := 16

	t.Logf("Key: %s", key)
	t.Logf("Hash (decimal): %d", hash(key))
	t.Logf("Hash (hexadecimal): %x", hash(key))
	t.Logf("Bucket: %d", bucketIndex(key, bucketCount))
}