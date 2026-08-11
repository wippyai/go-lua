package typevalue

import (
	"sync"
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

// Cache is the owner of all type-to-value memo tables, including the lazily
// created variant plane. Concurrent callers may share one cache, but no query
// may observe a partially initialized table or a mutable result alias.
func TestCacheOwnsConcurrentTypeValueQueries(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()

	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.String).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.Number).
		Build()
	union := typeexpr.Union(left, right)
	unknownRecord := typetable.NewRecord().Field("value", typ.Unknown).Build()

	// These immutable values are inputs only; every mutable cache plane starts
	// empty so this also exercises concurrent lazy initialization.
	unionValue := WithWitness(reg, FromType(reg, union), union)
	unknownValue := WithWitness(reg, FromType(reg, unknownRecord), unknownRecord)

	const workers = 8
	const rounds = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			defer wg.Done()
			<-start
			for round := 0; round < rounds; round++ {
				switch (worker + round) % 8 {
				case 0:
					cache.FromType(reg, union)
				case 1:
					cache.FromTypeWithWitness(reg, union)
				case 2:
					cache.FromType(reg, unknownRecord)
				case 3:
					cache.FromTypeWithWitness(reg, unknownRecord)
				case 4:
					cache.TypeOf(reg, unionValue)
				case 5:
					RuntimeTypeProfileOf(reg, cache, unknownValue)
				case 6:
					cache.Variants().OriginOfType(union)
				case 7:
					cache.HasConcreteType(reg, unionValue)
				}
			}
		}(worker)
	}
	close(start)
	wg.Wait()

	family, cases, ok := cache.Variants().OriginOfType(union)
	if !ok || family == 0 || len(cases) != 2 {
		t.Fatalf("cached variant origin = %d/%v/%v, want two owned cases", family, cases, ok)
	}
	// Origin queries return a fresh case slice. Mutating a consumer copy must
	// not poison the family retained by the cache.
	cases[0] = -1
	_, freshCases, ok := cache.Variants().OriginOfType(union)
	if !ok || len(freshCases) != 2 || freshCases[0] < 0 {
		t.Fatalf("variant cache retained mutable result alias: %v/%v", freshCases, ok)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	for name, initialized := range map[string]bool{
		"values":           len(cache.values) != 0,
		"witnesses":        len(cache.witnesses) != 0,
		"valuesByShape":    len(cache.valuesByShape) != 0,
		"witnessesByShape": len(cache.witnessesByShape) != 0,
		"typeProfiles":     len(cache.typeProfiles) != 0,
		"unknownTypes":     len(cache.unknownTypes) != 0,
		"variants":         cache.variants != nil,
	} {
		if !initialized {
			t.Errorf("concurrent law did not initialize owner table %s", name)
		}
	}
}
