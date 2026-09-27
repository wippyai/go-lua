package store

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewInterprocState(t *testing.T) {
	state := NewInterprocState()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Facts == nil {
		t.Error("Facts map should be initialized")
	}
	if state.Refinements == nil {
		t.Error("Refinements map should be initialized")
	}
	if state.ConstructorFields == nil {
		t.Error("ConstructorFields map should be initialized")
	}
}

func TestEffectsEqual_BothNil(t *testing.T) {
	if !effectsEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
}

func TestEffectsEqual_OneNil(t *testing.T) {
	eff := &constraint.FunctionRefinement{}
	if effectsEqual(eff, nil) {
		t.Error("non-nil and nil should not be equal")
	}
	if effectsEqual(nil, eff) {
		t.Error("nil and non-nil should not be equal")
	}
}

func TestEffectsEqual_Same(t *testing.T) {
	eff := &constraint.FunctionRefinement{Terminates: true}
	if !effectsEqual(eff, eff) {
		t.Error("same reference should be equal")
	}
}

func TestEffectsMapEqual_Empty(t *testing.T) {
	if !effectsMapEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
	if !effectsMapEqual(map[cfg.SymbolID]*constraint.FunctionRefinement{}, map[cfg.SymbolID]*constraint.FunctionRefinement{}) {
		t.Error("two empty maps should be equal")
	}
}

func TestEffectsMapEqual_DifferentLength(t *testing.T) {
	a := map[cfg.SymbolID]*constraint.FunctionRefinement{1: {}}
	b := map[cfg.SymbolID]*constraint.FunctionRefinement{}
	if effectsMapEqual(a, b) {
		t.Error("maps of different length should not be equal")
	}
}

func TestInterprocFactsMapEqual_Empty(t *testing.T) {
	if !interprocFactsMapEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
	if !interprocFactsMapEqual(map[api.GraphKey]api.Facts{}, map[api.GraphKey]api.Facts{}) {
		t.Error("two empty maps should be equal")
	}
}

func TestInterprocFactsMapEqual_DifferentLength(t *testing.T) {
	a := map[api.GraphKey]api.Facts{{GraphID: 1}: {}}
	b := map[api.GraphKey]api.Facts{}
	if interprocFactsMapEqual(a, b) {
		t.Error("maps of different length should not be equal")
	}
}

func TestWidenInterprocFacts_Empty(t *testing.T) {
	result := widenInterprocFacts(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Error("expected empty map")
	}
}

func TestWidenInterprocFacts_OnlyPrev(t *testing.T) {
	fn := &ast.FunctionExpr{}
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			Callables: api.Callables{
				fn: {Summary: []typ.Type{typ.String}},
			},
		},
	}
	result := widenInterprocFacts(prev, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_OnlyNext(t *testing.T) {
	fn := &ast.FunctionExpr{}
	next := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			Callables: api.Callables{
				fn: {Summary: []typ.Type{typ.Number}},
			},
		},
	}
	result := widenInterprocFacts(nil, next)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_Merge(t *testing.T) {
	fn := &ast.FunctionExpr{}
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			Callables: api.Callables{
				fn: {Summary: []typ.Type{typ.String}},
			},
		},
	}
	next := map[api.GraphKey]api.Facts{
		{GraphID: 2}: {
			Callables: api.Callables{
				fn: {Summary: []typ.Type{typ.Number}},
			},
		},
	}
	result := widenInterprocFacts(prev, next)
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestReturnSummariesFromFacts_FallsBackToCanonical(t *testing.T) {
	graph, fn := callableViewGraph(1)
	facts := api.Facts{
		Callables: api.Callables{
			fn: {
				Summary: []typ.Type{typ.String},
			},
		},
	}
	got := returns.SummaryViewFromFacts(graph, facts)
	if len(got) != 1 || len(got[cfg.SymbolID(1)]) != 1 || got[cfg.SymbolID(1)][0] != typ.String {
		t.Fatalf("unexpected summary view: %#v", got)
	}
}

func TestNarrowReturnSummariesFromFacts_FallsBackToCanonical(t *testing.T) {
	graph, fn := callableViewGraph(2)
	facts := api.Facts{
		Callables: api.Callables{
			fn: {
				Narrow: []typ.Type{typ.Number},
			},
		},
	}
	got := returns.NarrowViewFromFacts(graph, facts)
	if len(got) != 1 || len(got[cfg.SymbolID(2)]) != 1 || got[cfg.SymbolID(2)][0] != typ.Number {
		t.Fatalf("unexpected narrow view: %#v", got)
	}
}

func TestLocalFuncTypesFromFacts_FallsBackToCanonical(t *testing.T) {
	graph, literal := callableViewGraph(3)
	fn := typ.Func().Returns(typ.Boolean).Build()
	facts := api.Facts{
		Callables: api.Callables{
			literal: {
				Func: fn,
			},
		},
	}
	got := returns.FuncTypeViewFromFacts(graph, facts)
	if len(got) != 1 || !typ.TypeEquals(got[cfg.SymbolID(3)], fn) {
		t.Fatalf("unexpected func type view: %#v", got)
	}
}

func callableViewGraph(sym cfg.SymbolID) (*cfg.Graph, *ast.FunctionExpr) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	graph.Bindings().SetFuncLitSymbol(fn, sym)
	return graph, fn
}

func TestFunctionFactViewUsesStableSnapshotUntilSwap(t *testing.T) {
	graph, fn := callableViewGraph(42)
	parent := scope.New()
	s := NewSessionStore()
	key, ok := s.GraphKeyFor(graph, parent)
	if !ok {
		t.Fatal("missing graph key")
	}
	s.InterprocPrev.Facts[key] = api.Facts{Callables: api.Callables{
		fn: {Summary: []typ.Type{typ.String}},
	}}
	first := s.functionFactView(graph, parent)
	if first.summaries[42][0] != typ.String {
		t.Fatal("missing initial summary")
	}
	// A repeated read must reuse the fold while the stable snapshot is unchanged.
	if got := s.functionFactView(graph, parent); got.definitions[42].Summary[0] != typ.String {
		t.Fatal("missing cached definition")
	}
	if len(s.functionViews.views) != 1 {
		t.Fatal("expected one cached graph view")
	}
	s.InterprocNext.Facts[key] = api.Facts{Callables: api.Callables{
		fn: {Summary: []typ.Type{typ.Number}},
	}}
	if !s.FixpointSwap() {
		t.Fatal("expected changed facts")
	}
	if len(s.functionViews.views) != 0 {
		t.Fatal("cache survived facts swap")
	}
	if got := s.functionFactView(graph, parent); got.summaries[42][0] == typ.String {
		t.Fatal("stale summary after swap")
	}
}

func TestSessionStore_Fields(t *testing.T) {
	s := &SessionStore{
		Module: &ModuleStore{
			Graphs: make(map[uint64]*cfg.Graph),
		},
		Iteration: &IterationStore{
			Revision: 5,
		},
	}
	if s.Module == nil {
		t.Error("Module should be set")
	}
	if s.Iteration.Revision != 5 {
		t.Error("Revision should be 5")
	}
}

func TestModuleStore_Fields(t *testing.T) {
	m := &ModuleStore{
		Graphs:        make(map[uint64]*cfg.Graph),
		Parents:       make(map[uint64]*scope.State),
		ModuleAliases: map[cfg.SymbolID]string{1: "test"},
	}
	if m.ModuleAliases[1] != "test" {
		t.Error("ModuleAliases not set correctly")
	}
}

func TestFunctionRegistry_Fields(t *testing.T) {
	r := &FunctionRegistry{
		BySym:     make(map[cfg.SymbolID]*api.FunctionRef),
		ByFunc:    make(map[*ast.FunctionExpr]*api.FunctionRef),
		ByGraphID: make(map[uint64]*api.FunctionRef),
	}
	if r.BySym == nil {
		t.Error("BySym should be initialized")
	}
}

func TestIterationStore_Fields(t *testing.T) {
	i := &IterationStore{Revision: 10}
	if i.Revision != 10 {
		t.Error("Revision not set")
	}
}

func TestIterationScratch_Fields(t *testing.T) {
	s := &IterationScratch{
		SigsByGraphID: make(map[uint64]map[*ast.FunctionExpr]*typ.Function),
	}
	if s.SigsByGraphID == nil {
		t.Error("SigsByGraphID should be initialized")
	}
}

func TestFixpointSwap_TracksChannelDiffsAndResetsNext(t *testing.T) {
	s := NewSessionStore()
	fn := &ast.FunctionExpr{}

	s.InterprocNext.Refinements[1] = &constraint.FunctionRefinement{Terminates: true}
	s.InterprocNext.Facts[api.GraphKey{GraphID: 7, ParentHash: 11}] = api.Facts{
		Callables: api.Callables{
			fn: {Summary: []typ.Type{typ.String}},
		},
	}
	s.InterprocNext.ConstructorFields[3] = map[string]typ.Type{
		"v": typ.Number,
	}

	if !s.FixpointSwap() {
		t.Fatal("expected fixpoint swap to report changes")
	}

	diffs := s.FixpointChannelDiffs()
	if len(diffs) != 3 {
		t.Fatalf("expected 3 channel diffs, got %v", diffs)
	}
	if diffs[0] != "Refinements" || diffs[1] != "InterprocFacts" || diffs[2] != "ConstructorFields" {
		t.Fatalf("unexpected diff order/content: %v", diffs)
	}

	if len(s.InterprocPrev.Refinements) != 1 || s.InterprocPrev.Refinements[1] == nil {
		t.Fatalf("expected prev effects populated, got %#v", s.InterprocPrev.Refinements)
	}
	if len(s.InterprocNext.Refinements) != 0 {
		t.Fatalf("expected next effects reset, got %#v", s.InterprocNext.Refinements)
	}
	if len(s.InterprocPrev.Facts) != 1 {
		t.Fatalf("expected prev facts populated, got %#v", s.InterprocPrev.Facts)
	}
	if len(s.InterprocNext.Facts) != 0 {
		t.Fatalf("expected next facts reset, got %#v", s.InterprocNext.Facts)
	}
	if len(s.InterprocPrev.ConstructorFields) != 1 {
		t.Fatalf("expected prev constructor fields populated, got %#v", s.InterprocPrev.ConstructorFields)
	}
	if len(s.InterprocNext.ConstructorFields) != 0 {
		t.Fatalf("expected next constructor fields reset, got %#v", s.InterprocNext.ConstructorFields)
	}
}

func TestClearIterationChannels_InitializesMissingState(t *testing.T) {
	s := &SessionStore{}
	s.ClearIterationChannels()

	if s.Iteration == nil {
		t.Fatal("expected iteration store to be initialized")
	}
	if s.Scratch == nil {
		t.Fatal("expected scratch to be initialized")
	}
	if s.InterprocPrev == nil || s.InterprocNext == nil {
		t.Fatal("expected interproc states to be initialized")
	}
	if s.Scratch.SigsByGraphID == nil {
		t.Fatal("expected scratch literal signatures map to be initialized")
	}
}

func TestBumpRevision_InitializesIterationStore(t *testing.T) {
	s := &SessionStore{}
	s.BumpRevision()
	if got := s.Revision(); got != 1 {
		t.Fatalf("expected revision 1, got %d", got)
	}
}

func TestFixpointChannelDiffs_ReturnsCopy(t *testing.T) {
	s := NewSessionStore()
	s.StoreFunctionRefinement(1, &constraint.FunctionRefinement{Terminates: true})
	if !s.FixpointSwap() {
		t.Fatal("expected change from effect swap")
	}

	diffs := s.FixpointChannelDiffs()
	if len(diffs) == 0 {
		t.Fatal("expected non-empty diffs")
	}
	diffs[0] = "MUTATED"

	diffs2 := s.FixpointChannelDiffs()
	if len(diffs2) == 0 || diffs2[0] == "MUTATED" {
		t.Fatalf("expected defensive copy, got %v", diffs2)
	}
}

func TestClearIterationChannels_ResetsRevision(t *testing.T) {
	s := NewSessionStore()
	s.BumpRevision()
	s.BumpRevision()
	if got := s.Revision(); got != 2 {
		t.Fatalf("expected revision 2, got %d", got)
	}

	s.ClearIterationChannels()
	if got := s.Revision(); got != 0 {
		t.Fatalf("expected revision reset to 0, got %d", got)
	}
}

// Bindings of a class table within a round join their bodies, so a later
// snapshot refines the earlier ones; the next round binds a fresh snapshot of
// the same identity.
func TestBindClassSelf_JoinsBindingsWithinRound(t *testing.T) {
	chunk, err := parse.Parse(strings.NewReader("local C = {}\nreturn C"), "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk})
	var sym cfg.SymbolID
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if target, ok := info.FirstTarget(); ok && target.Symbol != 0 {
			sym = target.Symbol
		}
	})
	s := NewSessionStore()
	first := s.BindClassSelf(graph, 0, sym, "C", typ.NewRecord().Field("name", typ.String).Build())
	again := s.BindClassSelf(graph, 0, sym, "C", typ.NewRecord().Field("id", typ.Integer).Build())
	joined, ok := again.(*typ.Recursive)
	if !ok {
		t.Fatalf("expected a snapshot, got %s", again)
	}
	if body, ok := joined.Body.(*typ.Record); !ok || body.GetField("name") == nil || body.GetField("id") == nil {
		t.Fatalf("expected the round's bindings joined, got %s", typ.FormatShort(joined.Body))
	}

	s.FixpointSwap()
	next := s.BindClassSelf(graph, 0, sym, "C", typ.NewRecord().Field("id", typ.Integer).Build())
	a, _ := first.(*typ.Recursive)
	b, ok := next.(*typ.Recursive)
	if !ok || a == nil || b.ID != a.ID || b == a {
		t.Fatalf("expected a fresh snapshot of the same identity, got %s", next)
	}
	if body, ok := b.Body.(*typ.Record); !ok || body.GetField("id") == nil || body.GetField("name") != nil {
		t.Fatalf("expected the next round's body alone, got %s", typ.FormatShort(b.Body))
	}
}
