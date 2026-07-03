package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestResultQueryCachePathValuesUseInlineTierBeforeMap(t *testing.T) {
	var cache resultQueryCache
	value := cachedProductValue{value: product.Top(), ok: true}

	for i := 0; i < resultQueryInline; i++ {
		cache.pathValues.remember(pathValueCacheKey{point: cfg.Point(i + 1), path: testQueryCachePathIdentity(i + 1)}, value)
	}
	if cache.pathValues.inlineLen != resultQueryInline {
		t.Fatalf("inline path entries = %d, want %d", cache.pathValues.inlineLen, resultQueryInline)
	}
	if cache.pathValues.values != nil {
		t.Fatal("path cache allocated map before inline tier overflowed")
	}

	overflow := pathValueCacheKey{point: cfg.Point(99), path: testQueryCachePathIdentity(99)}
	cache.pathValues.remember(overflow, value)
	if cache.pathValues.inlineLen != 0 {
		t.Fatalf("inline path entries after overflow = %d, want 0", cache.pathValues.inlineLen)
	}
	if len(cache.pathValues.values) != resultQueryInline+1 {
		t.Fatalf("map path entries after overflow = %d, want %d", len(cache.pathValues.values), resultQueryInline+1)
	}
	if _, ok := cache.pathValues.lookup(overflow); !ok {
		t.Fatal("overflow path value was not readable after map spill")
	}

	cache.reset()
	if cache.pathValueCount() != 0 || cache.pathValues.values != nil || cache.pathValues.inlineLen != 0 {
		t.Fatal("reset did not clear path value cache")
	}
}

func TestResultQueryCacheSourceValuesUseInlineTierBeforeMap(t *testing.T) {
	var cache resultQueryCache
	value := cachedProductValue{value: product.Top(), ok: true}

	for i := 0; i < resultQueryInline; i++ {
		cache.sourceValues.remember(sourceValueCacheKey{point: cfg.Point(i + 1)}, value)
	}
	if cache.sourceValues.inlineLen != resultQueryInline {
		t.Fatalf("inline source entries = %d, want %d", cache.sourceValues.inlineLen, resultQueryInline)
	}
	if cache.sourceValues.values != nil {
		t.Fatal("source cache allocated map before inline tier overflowed")
	}

	overflow := sourceValueCacheKey{point: cfg.Point(99)}
	cache.sourceValues.remember(overflow, value)
	if cache.sourceValues.inlineLen != 0 {
		t.Fatalf("inline source entries after overflow = %d, want 0", cache.sourceValues.inlineLen)
	}
	if len(cache.sourceValues.values) != resultQueryInline+1 {
		t.Fatalf("map source entries after overflow = %d, want %d", len(cache.sourceValues.values), resultQueryInline+1)
	}
	if _, ok := cache.sourceValues.lookup(overflow); !ok {
		t.Fatal("overflow source value was not readable after map spill")
	}

	cache.reset()
	if cache.sourceValues.count() != 0 || cache.sourceValues.values != nil || cache.sourceValues.inlineLen != 0 {
		t.Fatal("reset did not clear source value cache")
	}
}

func testQueryCacheKey(index int) keyspace.Key {
	return keyspace.Key{Kind: keyspace.KindRetSlot, Root: uint32(index)}
}

func testQueryCachePathIdentity(index int) keyspace.PathIdentity {
	return keyspace.PathIdentity{Key: testQueryCacheKey(index)}
}
