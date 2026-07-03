package subtype

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCacheMemoizesAcyclicSubtypeQueries(t *testing.T) {
	cache := NewCache()
	sub := typetable.NewRecord().
		Field("id", typ.LiteralString("acct_1")).
		Field("count", typ.Integer).
		Build()
	super := typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()

	if !cache.IsSubtype(sub, super) {
		t.Fatal("first cached subtype query should match uncached semantics")
	}
	if len(cache.subtypes) != 1 {
		t.Fatalf("cached subtype entries = %d, want 1", len(cache.subtypes))
	}
	if !cache.IsSubtype(sub, super) {
		t.Fatal("second cached subtype query should reuse the same result")
	}
	if len(cache.subtypes) != 1 {
		t.Fatalf("cached subtype entries after repeat = %d, want 1", len(cache.subtypes))
	}
}

func TestCacheMemoizesFreshAssignableSeparately(t *testing.T) {
	cache := NewCache()
	sub := typetable.NewRecord().
		Field("status", typ.LiteralString("queued")).
		Field("error", typ.Nil).
		Build()
	super := typetable.NewRecord().
		Field("status", typeexpr.Union(typ.LiteralString("queued"), typ.LiteralString("done"))).
		Field("error", typeexpr.Optional(typ.String)).
		Build()

	if !cache.IsFreshAssignable(sub, super) {
		t.Fatal("cached fresh assignability should match uncached semantics")
	}
	if len(cache.freshAssignable) != 1 {
		t.Fatalf("cached fresh-assignable entries = %d, want 1", len(cache.freshAssignable))
	}
	if !cache.IsFreshAssignable(sub, super) {
		t.Fatal("second cached fresh assignability query should reuse the same result")
	}
	if len(cache.freshAssignable) != 1 {
		t.Fatalf("cached fresh-assignable entries after repeat = %d, want 1", len(cache.freshAssignable))
	}
}

func TestCacheDoesNotStoreRecursivePairs(t *testing.T) {
	cache := NewCache()
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Build()
	})

	if !cache.IsSubtype(node, node) {
		t.Fatal("recursive self-subtype should still be accepted")
	}
	if len(cache.subtypes) != 0 {
		t.Fatalf("recursive subtype queries must stay out of scoped cache, got %d entries", len(cache.subtypes))
	}
}
