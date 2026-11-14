package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/oussema-fatnassi/CacheLab/internal/cache"
)

// engineMetadata describes construction settings, not current memory usage.
type engineMetadata struct {
	Name, Hash, Locking string
	Preallocation       preallocationMetadata
	BranchCount         int
	MaxDepth            int
}

type preallocationMetadata struct {
	Policy             string `json:"policy"`
	InitialBucketCount *int   `json:"initial_bucket_count,omitempty"`
}

// newConfiguredTable reads the engine once. Only chained uses CACHE_BUCKET_COUNT.
// Table and metadata are built together so measurements describe this instance.
func newConfiguredTable() (cache.Table, engineMetadata, error) {
	name := os.Getenv("CACHE_ENGINE")
	if name == "" {
		name = "chained"
	}
	metadata := engineMetadata{Name: name, Hash: "fnv1a-64", Locking: "single-sync.Mutex-all-operations"}
	switch name {
	case "chained":
		buckets, err := positiveSetting("CACHE_BUCKET_COUNT", 16)
		if err != nil {
			return nil, engineMetadata{}, err
		}
		metadata.Preallocation = preallocationMetadata{Policy: "initial-bucket-slice", InitialBucketCount: &buckets}
		return cache.NewHashTable(buckets), metadata, nil
	case "recursive":
		config := cache.RecursiveConfig{BranchCount: 16, MaxDepth: 16}
		table, err := cache.NewRecursiveTableWithConfig(config)
		if err != nil {
			return nil, engineMetadata{}, err
		}
		metadata.BranchCount, metadata.MaxDepth = config.BranchCount, config.MaxDepth
		metadata.Preallocation.Policy = "nodes-on-demand"
		return table, metadata, nil
	case "go-map":
		metadata.Hash = "go-runtime"
		metadata.Preallocation.Policy = "no-explicit-capacity-hint"
		return cache.NewGoMap(), metadata, nil
	default:
		return nil, engineMetadata{}, fmt.Errorf("CACHE_ENGINE must be chained, recursive or go-map")
	}
}

// logExecutionMetadata never receives credentials or the full environment.
func logExecutionMetadata(logger *slog.Logger, metadata engineMetadata) {
	attributes := []any{
		"engine", metadata.Name,
		"server_go_version", runtime.Version(),
		"hash", metadata.Hash,
		"locking", metadata.Locking,
		"preallocation", metadata.Preallocation,
	}
	if metadata.BranchCount != 0 {
		attributes = append(attributes, "branch_count", metadata.BranchCount, "max_depth", metadata.MaxDepth)
	}
	logger.Info("execution metadata", attributes...)
}
