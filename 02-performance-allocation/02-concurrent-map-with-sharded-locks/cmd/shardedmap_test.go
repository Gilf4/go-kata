package main

import (
	"fmt"
	"testing"
)

func BenchmarkShardedMapSet(b *testing.B) {
	shards := 64
	b.Run(fmt.Sprintf("shards=%d", shards), func(b *testing.B) {
		sm := NewShardedMap[int, int](uint64(shards))

		keys := make([]int, b.N)
		for i := range keys {
			keys[i] = i
		}

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := keys[i%len(keys)]
				sm.Set(key, 42)
				i++
			}
		})
	})
}

func TestShardedMapSetAllocs(t *testing.T) {
	sm := NewShardedMap[int, int](64)
	key := 42
	value := 100

	allocs := testing.AllocsPerRun(1000, func() {
		sm.Set(key, value)
	})

	if allocs > 0 {
		t.Errorf("Expected 0 allocations, got %f", allocs)
	}
	fmt.Printf("Allocs per Set: %f\n", allocs)
}
