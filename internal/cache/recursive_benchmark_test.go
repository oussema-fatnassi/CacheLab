package cache

import (
	"fmt"
	"testing"
)

func BenchmarkRecursiveSubdivision(b *testing.B) {
	// Restore a root leaf between iterations, so each measured insertion
	// creates three branches. Fixture maintenance is outside the timed region.
	table, err := NewRecursiveTableWithConfig(RecursiveConfig{Hash: func(key string) uint64 {
		if key == "b" {
			return 0x100
		}
		return 0
	}})
	if err != nil {
		b.Fatal(err)
	}
	if err := table.Create("a", "value"); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	depth := 0
	for i := 0; i < b.N; i++ {
		if err := table.Create("b", "value"); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if i == b.N-1 {
			depth = table.Stats().MaxDepth
		}
		// Remove both entries because nonempty branches are not compacted.
		table.Delete("a")
		table.Delete("b")
		if err := table.Create("a", "value"); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
	b.StopTimer()
	b.ReportMetric(float64(depth), "depth")
}

func BenchmarkRecursiveRead(b *testing.B) {
	for _, collision := range []bool{false, true} {
		b.Run(fmt.Sprintf("full-collision=%t", collision), func(b *testing.B) {
			config := RecursiveConfig{}
			if collision {
				config.Hash = func(string) uint64 { return 7 }
			}
			table, err := NewRecursiveTableWithConfig(config)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < 128; i++ {
				if err := table.Create(fmt.Sprintf("key-%d", i), "value"); err != nil {
					b.Fatal(err)
				}
			}
			depth := table.Stats().MaxDepth
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if value, found := table.Read("key-127"); !found || value != "value" {
					b.Fatal("missing value")
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(depth), "depth")
		})
	}
}
