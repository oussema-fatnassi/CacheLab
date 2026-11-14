package cache

import (
	"fmt"
	"math/bits"
	"sync"
)

// RecursiveConfig is copied at construction; zero values select defaults.
// Key/value byte limits belong to NewCacheService, not the storage engine.
type RecursiveConfig struct {
	// Hash defaults to FNV-1a. It must be deterministic and must not call back
	// into this table. A shared custom hash must be safe across table instances.
	Hash func(string) uint64
	// BranchCount is a power of two from 2 to 256; default 16. The upper bound
	// limits each branch to at most 256 child pointers (8 hash bits per level).
	BranchCount int
	// MaxDepth limits the number of hash-bit selections. Default: enough levels
	// to consume 64 bits. A smaller limit keeps remaining collisions in a leaf.
	MaxDepth int
}

type recursiveEntry struct {
	key, value string
	hash       uint64
}

// A node is either a branch (children != nil) or a leaf containing entries.
// Only branches allocate a child slice; leaves start with one entry.
type recursiveNode struct {
	children []*recursiveNode
	entries  []recursiveEntry
}

// RecursiveTable routes hashes from low to high bits. One mutex protects all
// operations, including reads and statistics. Construct it with a constructor
// and do not copy it after first use.
type RecursiveTable struct {
	mu           sync.Mutex
	root         *recursiveNode
	config       RecursiveConfig
	bitsPerLevel int
	size         int
	subdivisions int
}

var _ Table = (*RecursiveTable)(nil)

// NewRecursiveTable uses FNV-1a, 16 branches and at most 16 selection levels.
func NewRecursiveTable() *RecursiveTable {
	table, _ := NewRecursiveTableWithConfig(RecursiveConfig{})
	return table
}

// NewRecursiveTableWithConfig rejects invalid settings before allocating nodes.
// A final partial group uses the remaining high bits; unused children stay nil.
func NewRecursiveTableWithConfig(config RecursiveConfig) (*RecursiveTable, error) {
	if config.Hash == nil {
		config.Hash = hash
	}
	if config.BranchCount == 0 {
		config.BranchCount = 16
	}
	if config.BranchCount < 2 || config.BranchCount > 256 || config.BranchCount&(config.BranchCount-1) != 0 {
		return nil, fmt.Errorf("branch count must be a power of two from 2 to 256")
	}
	bitsPerLevel := bits.Len(uint(config.BranchCount)) - 1
	levels := (64 + bitsPerLevel - 1) / bitsPerLevel
	if config.MaxDepth == 0 {
		config.MaxDepth = levels
	}
	if config.MaxDepth < 1 || config.MaxDepth > levels {
		return nil, fmt.Errorf("max depth must be between 1 and %d", levels)
	}
	return &RecursiveTable{config: config, bitsPerLevel: bitsPerLevel}, nil
}

// childIndex is called only for branches below MaxDepth, so the shift is < 64.
func (table *RecursiveTable) childIndex(hashValue uint64, depth int) int {
	return int((hashValue >> (depth * table.bitsPerLevel)) & uint64(table.config.BranchCount-1))
}

// Create inserts a key without overwriting an existing value.
func (table *RecursiveTable) Create(key, value string) error {
	table.mu.Lock()
	defer table.mu.Unlock()
	entry := recursiveEntry{key: key, value: value, hash: table.config.Hash(key)}
	if err := table.insert(&table.root, entry, 0); err != nil {
		return err
	}
	table.size++
	return nil
}

// insert holds the caller's lock and reuses stored hashes during subdivision.
func (table *RecursiveTable) insert(link **recursiveNode, entry recursiveEntry, depth int) error {
	if *link == nil {
		*link = &recursiveNode{entries: []recursiveEntry{entry}}
		return nil
	}
	node := *link
	if node.children == nil {
		for _, existing := range node.entries {
			if existing.key == entry.key {
				return ErrKeyExists
			}
		}
		// Before the depth limit, all entries in a leaf have the same hash.
		// At the limit, a leaf may also hold distinct hashes with a common prefix.
		if depth == table.config.MaxDepth || node.entries[0].hash == entry.hash {
			node.entries = append(node.entries, entry)
			return nil
		}
		branch := &recursiveNode{children: make([]*recursiveNode, table.config.BranchCount)}
		branch.children[table.childIndex(node.entries[0].hash, depth)] = node
		*link = branch
		node = branch
		table.subdivisions++
	}
	index := table.childIndex(entry.hash, depth)
	return table.insert(&node.children[index], entry, depth+1)
}

// find returns the leaf entry under the caller's lock, or nil if absent.
func (table *RecursiveTable) find(key string, hashValue uint64) *recursiveEntry {
	node := table.root
	for depth := 0; node != nil; depth++ {
		if node.children == nil {
			for i := range node.entries {
				if node.entries[i].key == key {
					return &node.entries[i]
				}
			}
			return nil
		}
		node = node.children[table.childIndex(hashValue, depth)]
	}
	return nil
}

// Read returns the exact value and whether the key exists.
func (table *RecursiveTable) Read(key string) (string, bool) {
	table.mu.Lock()
	defer table.mu.Unlock()
	entry := table.find(key, table.config.Hash(key))
	if entry == nil {
		return "", false
	}
	return entry.value, true
}

// Update changes only an existing key's value.
func (table *RecursiveTable) Update(key, value string) error {
	table.mu.Lock()
	defer table.mu.Unlock()
	entry := table.find(key, table.config.Hash(key))
	if entry == nil {
		return ErrNotFound
	}
	entry.value = value
	return nil
}

// Delete prunes empty branches. Nonempty branches are not compacted, even when
// only one child remains, keeping the routing rule simple and predictable.
func (table *RecursiveTable) Delete(key string) bool {
	table.mu.Lock()
	defer table.mu.Unlock()
	if !table.remove(&table.root, key, table.config.Hash(key), 0) {
		return false
	}
	table.size--
	return true
}

func (table *RecursiveTable) remove(link **recursiveNode, key string, hashValue uint64, depth int) bool {
	node := *link
	if node == nil {
		return false
	}
	if node.children == nil {
		for i, entry := range node.entries {
			if entry.key == key {
				copy(node.entries[i:], node.entries[i+1:])
				last := len(node.entries) - 1
				node.entries[last] = recursiveEntry{} // Release the removed strings.
				node.entries = node.entries[:last]
				if last == 0 {
					*link = nil
				}
				return true
			}
		}
		return false
	}
	index := table.childIndex(hashValue, depth)
	if !table.remove(&node.children[index], key, hashValue, depth+1) {
		return false
	}
	for _, child := range node.children {
		if child != nil {
			return true
		}
	}
	*link = nil
	return true
}

// RecursiveStats describes the current tree, except Subdivisions which counts
// all branches created since construction. It is not a heap allocation count.
type RecursiveStats struct {
	Entries, Branches, MaxDepth, Subdivisions int
}

// Stats walks the tree under the mutex. MaxDepth counts hash-bit selections on
// the deepest occupied path (zero for an empty table or a root leaf).
// Call outside timed CRUD benchmarks; use -benchmem for actual allocations.
func (table *RecursiveTable) Stats() RecursiveStats {
	table.mu.Lock()
	defer table.mu.Unlock()
	stats := RecursiveStats{Entries: table.size, Subdivisions: table.subdivisions}
	var visit func(*recursiveNode, int)
	visit = func(node *recursiveNode, depth int) {
		if node == nil {
			return
		}
		stats.MaxDepth = max(stats.MaxDepth, depth)
		if node.children != nil {
			stats.Branches++
			for _, child := range node.children {
				visit(child, depth+1)
			}
		}
	}
	visit(table.root, 0)
	return stats
}
