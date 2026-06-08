package paramboundary

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestParameterSlotsFromSlotsNormalizesLookup(t *testing.T) {
	first := cfg.SymbolID(7)
	second := cfg.SymbolID(3)

	got := ParameterSlotsFromSlots([]cfg.ParamSlot{
		{Symbol: 0},
		{Symbol: first, DeclPoint: 11},
		{Symbol: second, DeclPoint: 13},
	})

	if got.IsEmpty() {
		t.Fatal("parameter slot lookup should not be empty")
	}
	slot, ok := got.Lookup(second)
	if !ok || slot.Index != 2 || slot.DeclPoint != 13 {
		t.Fatalf("Lookup(%d) = %#v/%v, want index 2 decl 13", second, slot, ok)
	}
	if _, ok := got.Lookup(0); ok {
		t.Fatal("Lookup(0) unexpectedly succeeded")
	}
}

func TestParameterSlotsFromSymbolsUsesSlotOrder(t *testing.T) {
	first := cfg.SymbolID(5)
	second := cfg.SymbolID(8)

	got := ParameterSlotsFromSymbols([]cfg.SymbolID{first, 0, second})

	slot, ok := got.Lookup(first)
	if !ok || slot.Index != 0 {
		t.Fatalf("Lookup(first) = %#v/%v, want index 0", slot, ok)
	}
	slot, ok = got.Lookup(second)
	if !ok || slot.Index != 2 {
		t.Fatalf("Lookup(second) = %#v/%v, want index 2", slot, ok)
	}
	if _, ok := got.Lookup(0); ok {
		t.Fatal("Lookup(0) unexpectedly succeeded")
	}
}

func TestUnannotatedRootsFromFactsExcludesExplicitAny(t *testing.T) {
	untyped := cfg.SymbolID(1)
	explicitAny := cfg.SymbolID(2)
	declared := cfg.SymbolID(3)

	got := UnannotatedRootsFromFacts(
		[]cfg.SymbolID{untyped, explicitAny, declared},
		map[cfg.SymbolID]typ.Type{
			explicitAny: typ.Any,
			declared:    typ.String,
		},
		map[cfg.SymbolID]bool{explicitAny: true},
	)

	if !got.Contains(untyped) {
		t.Fatalf("untyped parameter missing from dynamic defaults: %v", got.Slice())
	}
	if got.Contains(explicitAny) {
		t.Fatalf("explicit any parameter became a dynamic default: %v", got.Slice())
	}
	if got.Contains(declared) {
		t.Fatalf("declared parameter became a dynamic default: %v", got.Slice())
	}
}

func TestUnannotatedRootsBySlotUsesCanonicalSlotAlignment(t *testing.T) {
	self := cfg.SymbolID(10)
	arg := cfg.SymbolID(11)

	got := UnannotatedRootsBySlot(
		[]cfg.SymbolID{self, arg},
		map[int]typ.Type{0: typ.NewRecord().Build()},
	)

	if got.Contains(self) {
		t.Fatalf("declared slot self became a dynamic default: %v", got.Slice())
	}
	if !got.Contains(arg) {
		t.Fatalf("unannotated arg missing from dynamic defaults: %v", got.Slice())
	}
}

func TestSourceUnannotatedReadsCanonicalParamSlots(t *testing.T) {
	graph := paramBoundaryGraph(t, `local function run(raw, typed: string, trusted: any) end`)
	nested := graph.NestedFunctions()[0].Func
	child := cfg.BuildWithBindings(nested, graph.Bindings())
	slots := child.ParamSlotsReadOnly()
	if len(slots) != 3 {
		t.Fatalf("param slots = %#v, want 3", slots)
	}

	if !SourceUnannotated(child, slots[0].Symbol, nil) {
		t.Fatal("raw parameter should be source-unannotated")
	}
	if SourceUnannotated(child, slots[1].Symbol, nil) {
		t.Fatal("typed parameter should not be source-unannotated")
	}
	if SourceUnannotated(child, slots[2].Symbol, nil) {
		t.Fatal("trusted:any parameter should not be source-unannotated")
	}
	if SourceUnannotated(child, slots[0].Symbol, func(cfg.SymbolID) bool { return true }) {
		t.Fatal("annotation fact veto should suppress source-unannotated status")
	}
}

func paramBoundaryGraph(t *testing.T, src string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(src, "paramboundary.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	graph := cfg.Build(root)
	if graph == nil || graph.Bindings() == nil || len(graph.NestedFunctions()) == 0 {
		t.Fatal("expected graph with nested function")
	}
	return graph
}
