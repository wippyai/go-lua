package cir

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestAddressResolverModesDistinguishCells(t *testing.T) {
	// A path operand resolves to a distinct cell per access mode; a non-path
	// operand has no addressable cell.
	fake := &fakeResolver{}
	pathOp := Operand{Kind: OperandPath, Ref: 1}
	tempOp := Operand{Kind: OperandTemp, Ref: 0}

	modes := []AccessMode{AccessReadBefore, AccessWriteLocal, AccessRootOrVisible, AccessEvidence}
	seen := map[keyspace.Key]AccessMode{}
	for _, m := range modes {
		key, ok := fake.Resolve(cfg.Point(3), pathOp, m)
		if !ok {
			t.Fatalf("mode %s: path operand must resolve", m)
		}
		if prev, dup := seen[key]; dup {
			t.Fatalf("mode %s collides with mode %s on key %v", m, prev, key)
		}
		seen[key] = m
	}
	if _, ok := fake.Resolve(cfg.Point(3), tempOp, AccessReadBefore); ok {
		t.Fatalf("temp operand must not resolve to a state cell")
	}
}

func TestCachingResolverMemoizesByPointOperandMode(t *testing.T) {
	inner := &fakeResolver{}
	caching := NewCachingResolver(inner)
	pathOp := Operand{Kind: OperandPath, Ref: 2}

	k1, ok1 := caching.Resolve(cfg.Point(1), pathOp, AccessReadBefore)
	k2, ok2 := caching.Resolve(cfg.Point(1), pathOp, AccessReadBefore)
	if !ok1 || !ok2 || k1 != k2 {
		t.Fatalf("cache returned inconsistent result: %v/%v %v/%v", k1, ok1, k2, ok2)
	}
	if inner.calls != 1 {
		t.Fatalf("caching resolver must call inner once for equal args, got %d", inner.calls)
	}

	// A different mode is a distinct cache slot and reaches the inner resolver.
	caching.Resolve(cfg.Point(1), pathOp, AccessWriteLocal)
	if inner.calls != 2 {
		t.Fatalf("distinct mode must miss the cache, got %d inner calls", inner.calls)
	}
}

// fakeResolver fabricates a deterministic distinct key per (point, op, mode) for
// path operands and counts inner invocations.
type fakeResolver struct {
	calls int
}

func (r *fakeResolver) Resolve(point cfg.Point, op Operand, mode AccessMode) (keyspace.Key, bool) {
	r.calls++
	if op.Kind != OperandPath {
		return keyspace.Key{}, false
	}
	// Encode the tuple into distinct comparable fields; the concrete encoding is
	// irrelevant, only distinctness and stability matter for the contract.
	return keyspace.Key{Sym: 1, Root: uint32(point), Ver: uint32(mode) + 1}, true
}
