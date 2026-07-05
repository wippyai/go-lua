package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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

func TestPathValueCacheKeyUsesKeyspaceIdentity(t *testing.T) {
	ks := keyspace.New()
	p := pathdom.Path{Root: "item", Symbol: symbol.ID(7), Version: 2}.Field("name")

	got, ok := newPathValueCacheKey(ks, sourceValueReadBoundary, cfg.Point(9), p)
	if !ok {
		t.Fatal("newPathValueCacheKey failed")
	}
	if got.mode != sourceValueReadBoundary || got.point != cfg.Point(9) {
		t.Fatalf("cache key = %#v, want mode/point preserved", got)
	}
	if got.path.Key.Kind == keyspace.KindInvalid || got.path.Legacy != "" {
		t.Fatalf("cache key path identity = %#v, want keyspace-backed identity", got.path)
	}
}

func TestDominatingMemberReadPresenceKeyUsesSamePathIdentityPolicy(t *testing.T) {
	ks := keyspace.New()
	p := pathdom.Path{Root: "item", Symbol: symbol.ID(8), Version: 1}.Field("ready")

	got, ok := newDominatingMemberReadPresenceKey(ks, cfg.Point(4), p)
	if !ok {
		t.Fatal("newDominatingMemberReadPresenceKey failed")
	}
	if got.point != cfg.Point(4) || got.path.Key.Kind == keyspace.KindInvalid || got.path.Legacy != "" {
		t.Fatalf("presence key = %#v, want keyspace-backed identity at point 4", got)
	}
}

func testQueryCacheKey(index int) keyspace.Key {
	return keyspace.Key{Kind: keyspace.KindRetSlot, Root: uint32(index)}
}

func testQueryCachePathIdentity(index int) keyspace.PathIdentity {
	return keyspace.PathIdentity{Key: testQueryCacheKey(index)}
}
