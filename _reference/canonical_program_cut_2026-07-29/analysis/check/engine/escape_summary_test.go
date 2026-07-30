package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/domain/placement"
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
	case placement.Store, placement.Retain:
	default:
		t.Fatalf("store_item param 0 escape = %v, want EscapeStore or EscapeRetain", store[0].EscapeClass)
	}
	if store[0].PlacementConsequence != placement.ConsequenceOwned {
		t.Fatalf("store_item param 0 placement = %q, want owned-heap", store[0].PlacementConsequence)
	}

	read, found := result.FunctionEscapes["read_item"]
	if !found || len(read) != 1 {
		t.Fatalf("read_item escapes = %#v, want one parameter relation", read)
	}
	if read[0].Param != 0 {
		t.Fatalf("read_item relation param = %d, want 0", read[0].Param)
	}
	if read[0].EscapeClass != placement.Borrow {
		t.Fatalf("read_item param 0 escape = %v, want EscapeBorrow", read[0].EscapeClass)
	}
	if read[0].PlacementConsequence != placement.Keep {
		t.Fatalf("read_item param 0 placement = %q, want keep", read[0].PlacementConsequence)
	}
}

// TestFunctionEscapesNamesTheStoreContainerFormal proves the summary recovers
// the owner position of a positional store from placement facts alone. The
// store boundary retains both formals under one operation and owns only the
// stored graph, so the container is the other formal that same operation
// retains. store_item names its box formal; keep_item writes into
// module-captured state and has no owner formal, so it stays ownerless rather
// than adopting an unrelated parameter.
func TestFunctionEscapesNamesTheStoreContainerFormal(t *testing.T) {
	result, err := engine.Check(`type Item = { id: string }
type Box = { label: string }
local saved: {[string]: Item} = {}
local function store_item(item: Item, box: Box) ownership.store(item, box) end
local function keep_item(item: Item, tag: Box) saved.last = item end
return { store_item = store_item, keep_item = keep_item }`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	store, found := result.FunctionEscapes["store_item"]
	if !found || len(store) != 2 {
		t.Fatalf("store_item escapes = %#v, want two parameter relations", store)
	}
	if store[0].EscapeClass != placement.Store {
		t.Fatalf("store_item param 0 escape = %v, want EscapeStore", store[0].EscapeClass)
	}
	if !store[0].HasStoredInto || store[0].StoredInto != 1 {
		t.Fatalf("store_item param 0 storedInto = (%v, %d), want (true, 1)", store[0].HasStoredInto, store[0].StoredInto)
	}
	if store[1].HasStoredInto {
		t.Fatalf("store_item param 1 storedInto = %d, want no container for the owner formal", store[1].StoredInto)
	}

	keep, found := result.FunctionEscapes["keep_item"]
	if !found || len(keep) != 2 {
		t.Fatalf("keep_item escapes = %#v, want two parameter relations", keep)
	}
	if keep[0].EscapeClass != placement.Store {
		t.Fatalf("keep_item param 0 escape = %v, want EscapeStore", keep[0].EscapeClass)
	}
	if keep[0].HasStoredInto {
		t.Fatalf("keep_item param 0 storedInto = %d, want no owner formal for a module-captured store", keep[0].StoredInto)
	}
}
