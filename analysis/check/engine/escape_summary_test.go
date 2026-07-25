package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// TestFunctionEscapesSummaryFromPlacementFacts proves the engine derives a
// per-exported-function per-parameter escape summary purely from the placement
// facts published by evaluating each exported body once with its formals seeded
// as allocation identities. store_item stores its parameter into module-captured
// state, so the parameter escapes as an owned store; read_item only projects a
// member of its parameter, so the parameter stays a borrow. No exporter or
// diagnostic change is involved: the summary is read from engine.Result alone.
func TestFunctionEscapesSummaryFromPlacementFacts(t *testing.T) {
	result, err := engine.Check(`type Item = { id: string, child: { meta: { route: string } } }
local saved: {[string]: Item} = {}
local function store_item(item: Item) saved.last = item end
local function read_item(item: Item): string return item.child.meta.route end
return { store_item = store_item, read_item = read_item }`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	store, found := result.FunctionEscapes["store_item"]
	if !found || len(store) != 1 {
		t.Fatalf("store_item escapes = %#v, want one parameter relation", store)
	}
	if store[0].Param != 0 {
		t.Fatalf("store_item relation param = %d, want 0", store[0].Param)
	}
	switch store[0].EscapeClass {
	case signature.EscapeStore, signature.EscapeRetain:
	default:
		t.Fatalf("store_item param 0 escape = %v, want EscapeStore or EscapeRetain", store[0].EscapeClass)
	}
	if store[0].PlacementConsequence != signature.PlacementConsequenceOwnedHeap {
		t.Fatalf("store_item param 0 placement = %q, want owned-heap", store[0].PlacementConsequence)
	}

	read, found := result.FunctionEscapes["read_item"]
	if !found || len(read) != 1 {
		t.Fatalf("read_item escapes = %#v, want one parameter relation", read)
	}
	if read[0].Param != 0 {
		t.Fatalf("read_item relation param = %d, want 0", read[0].Param)
	}
	if read[0].EscapeClass != signature.EscapeBorrow {
		t.Fatalf("read_item param 0 escape = %v, want EscapeBorrow", read[0].EscapeClass)
	}
	if read[0].PlacementConsequence != signature.PlacementConsequenceKeep {
		t.Fatalf("read_item param 0 placement = %q, want keep", read[0].PlacementConsequence)
	}
}
