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
	return testFlowAddressKey(t, key)
}

func testFlowAddressKey(t *testing.T, key constraint.PathKey) flow.StableAddress {
	t.Helper()
	addr, ok := flow.StableAddressFromCanonicalKey(key)
	if !ok {
		t.Fatalf("flow address for key %s", key)
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

func testKeyPresenceHas(t *testing.T, facts flow.KeyPresenceFacts, table, key constraint.Path) bool {
	t.Helper()
	return facts.HasAddresses(testFlowPathAddress(t, table), testFlowPathAddress(t, key))
}

func testKeyPresenceHasValue(t *testing.T, facts flow.KeyPresenceFacts, table, key, value constraint.Path) bool {
	t.Helper()
	return facts.HasValueAddresses(testFlowPathAddress(t, table), testFlowPathAddress(t, key), testFlowPathAddress(t, value))
}

func testKeyPresenceWith(t *testing.T, facts flow.KeyPresenceFacts, table, key constraint.Path) flow.KeyPresenceFacts {
	t.Helper()
	return facts.WithAddresses(testFlowPathAddress(t, table), testFlowPathAddress(t, key))
}

func testKeyPresenceWithValue(t *testing.T, facts flow.KeyPresenceFacts, table, key, value constraint.Path) flow.KeyPresenceFacts {
	t.Helper()
	return facts.WithValueAddresses(testFlowPathAddress(t, table), testFlowPathAddress(t, key), testFlowPathAddress(t, value))
}

func testKeyPresenceWithKeyArray(t *testing.T, facts flow.KeyPresenceFacts, array, table constraint.Path) flow.KeyPresenceFacts {
	t.Helper()
	return facts.WithKeyArrayAddresses(testFlowPathAddress(t, array), testFlowPathAddress(t, table))
}

func testKeyPresenceWithEmptyKeyArray(t *testing.T, facts flow.KeyPresenceFacts, array constraint.Path) flow.KeyPresenceFacts {
	t.Helper()
	return facts.WithEmptyKeyArrayAddress(testFlowPathAddress(t, array))
}

func testKeyPresenceWithAppendHistoryBase(t *testing.T, facts flow.KeyPresenceFacts, array constraint.Path) flow.KeyPresenceFacts {
	t.Helper()
	return facts.WithAppendHistoryBaseAddress(testFlowPathAddress(t, array))
}

func testKeyPresenceWithAppendElementFieldOrigin(
	t *testing.T,
	facts flow.KeyPresenceFacts,
	array constraint.Path,
	field []constraint.Segment,
	source constraint.Path,
) flow.KeyPresenceFacts {
	t.Helper()
	return facts.WithAppendElementFieldOriginAddresses(testFlowPathAddress(t, array), field, testFlowPathAddress(t, source))
}

func testStaticMemberValue(t *testing.T, facts flow.StaticMemberFacts, sym cfg.SymbolID, segs []constraint.Segment) (product.AbstractValue, bool) {
	t.Helper()
	return facts.ValueAtAddress(testStaticMemberAddress(t, sym, segs))
}

func testStaticMemberValueKey(t *testing.T, facts flow.StaticMemberFacts, key constraint.PathKey) (product.AbstractValue, bool) {
	t.Helper()
	return facts.ValueAtAddress(testStaticMemberAddressKey(t, key))
}
