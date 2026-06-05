package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

func testStaticMemberAddress(t *testing.T, sym cfg.SymbolID, segs []constraint.Segment) flow.StableAddress {
	t.Helper()
	addr, ok := flow.StableAddressOfSymbol(sym, segs)
	if !ok {
		t.Fatalf("static member address for symbol %d path %v", sym, segs)
	}
	return addr
}

func testStaticMemberAddressKey(t *testing.T, key constraint.PathKey) flow.StableAddress {
	t.Helper()
	addr, ok := flow.StableAddressFromKey(key)
	if !ok {
		t.Fatalf("static member address for %s", key)
	}
	return addr
}

func testFlowPathAddress(t *testing.T, path constraint.Path) flow.StableAddress {
	t.Helper()
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		t.Fatalf("flow address for path %s", path.Key())
	}
	return addr
}

func testStaticMemberValue(t *testing.T, facts flow.StaticMemberFacts, sym cfg.SymbolID, segs []constraint.Segment) (product.AbstractValue, bool) {
	t.Helper()
	return facts.ValueAtAddress(testStaticMemberAddress(t, sym, segs))
}

func testStaticMemberValueKey(t *testing.T, facts flow.StaticMemberFacts, key constraint.PathKey) (product.AbstractValue, bool) {
	t.Helper()
	return facts.ValueAtAddress(testStaticMemberAddressKey(t, key))
}
