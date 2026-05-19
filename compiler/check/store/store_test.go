package store

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
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
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.String}},
			},
		},
	}
	result := widenInterprocFacts(prev, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_OnlyNext(t *testing.T) {
	next := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.Number}},
			},
		},
	}
	result := widenInterprocFacts(nil, next)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_NormalizesNewFacts(t *testing.T) {
	fn := typ.Func().Param("value", typ.Unknown).Build()
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	next := map[api.GraphKey]api.Facts{
		key: {
			CapturedFields: api.CapturedFieldAssigns{
				cfg.SymbolID(10): {
					cfg.SymbolID(20): {
						"after_all": typ.NewOptional(fn),
					},
				},
			},
		},
	}

	result := widenInterprocFacts(nil, next)
	got := result[key].CapturedFields[cfg.SymbolID(10)][cfg.SymbolID(20)]["after_all"]
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("expected new facts to be normalized through WidenFacts, got %v", got)
	}
}

func TestWidenInterprocFacts_Merge(t *testing.T) {
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.String}},
			},
		},
	}
	next := map[api.GraphKey]api.Facts{
		{GraphID: 2}: {
			FunctionFacts: api.FunctionFacts{
				1: {Summary: []typ.Type{typ.Number}},
			},
		},
	}
	result := widenInterprocFacts(prev, next)
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestFunctionFactsSummaryAccessor(t *testing.T) {
	facts := api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {
				Summary: []typ.Type{typ.String},
			},
		},
	}
	got := facts.FunctionFacts.Summary(cfg.SymbolID(1))
	if len(got) != 1 || got[0] != typ.String {
		t.Fatalf("unexpected summary: %#v", got)
	}
}

func TestFunctionFactsNarrowAccessor(t *testing.T) {
	facts := api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(2): {
				Narrow: []typ.Type{typ.Number},
			},
		},
	}
	got := facts.FunctionFacts.NarrowSummary(cfg.SymbolID(2))
	if len(got) != 1 || got[0] != typ.Number {
		t.Fatalf("unexpected narrow summary: %#v", got)
	}
}

func TestFunctionFactsTypeAccessor(t *testing.T) {
	fn := typ.Func().Returns(typ.Boolean).Build()
	facts := api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(3): {
				Type: fn,
			},
		},
	}
	got := facts.FunctionFacts.FunctionType(cfg.SymbolID(3))
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("unexpected function type: %#v", got)
	}
}

func TestGetInterprocFactsSnapshot_UsesStoredGraphParentHash(t *testing.T) {
	graph := cfg.Build(&ast.FunctionExpr{})
	if graph == nil || graph.ID() == 0 {
		t.Fatal("expected graph with stable ID")
	}

	storedParent := scope.New().WithType("T", typ.String)
	currentParent := scope.New().WithType("T", typ.Number)
	if storedParent.Hash() == currentParent.Hash() {
		t.Fatal("test requires different parent hashes")
	}

	s := NewSessionStore()
	s.SetGraphParentHash(graph.ID(), storedParent.Hash())
	s.SetParentScope(storedParent.Hash(), storedParent)
	key := api.KeyForGraph(graph, storedParent.Hash())
	s.InterprocPrev.Facts[key] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {Summary: []typ.Type{typ.String}},
		},
	}

	got := s.GetInterprocFactsSnapshot(graph, currentParent)
	summary := got.FunctionFacts.Summary(cfg.SymbolID(1))
	if len(summary) != 1 || !typ.TypeEquals(summary[0], typ.String) {
		t.Fatalf("expected snapshot from stored parent hash, got %#v", summary)
	}
}

func TestGetInterprocFactsSnapshot_OverlaysCurrentIterationFacts(t *testing.T) {
	graph := cfg.Build(&ast.FunctionExpr{})
	if graph == nil || graph.ID() == 0 {
		t.Fatal("expected graph with stable ID")
	}

	parent := scope.New().WithType("T", typ.String)
	s := NewSessionStore()
	s.SetGraphParentHash(graph.ID(), parent.Hash())
	s.SetParentScope(parent.Hash(), parent)
	key := api.KeyForGraph(graph, parent.Hash())
	s.InterprocPrev.Facts[key] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {Summary: []typ.Type{typ.String}},
		},
	}
	s.InterprocNext.Facts[key] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {Summary: []typ.Type{typ.Number}},
		},
	}

	got := s.GetInterprocFactsSnapshot(graph, parent)
	summary := got.FunctionFacts.Summary(cfg.SymbolID(1))
	want := typ.NewUnion(typ.String, typ.Number)
	if len(summary) != 1 || !typ.TypeEquals(summary[0], want) {
		t.Fatalf("expected widened current snapshot %v, got %#v", want, summary)
	}
}

func TestGetInterprocFactsSnapshot_ReturnsImmutableFactContainers(t *testing.T) {
	graph := cfg.Build(&ast.FunctionExpr{})
	if graph == nil || graph.ID() == 0 {
		t.Fatal("expected graph with stable ID")
	}

	parent := scope.New().WithType("T", typ.String)
	s := NewSessionStore()
	s.SetGraphParentHash(graph.ID(), parent.Hash())
	s.SetParentScope(parent.Hash(), parent)
	key := api.KeyForGraph(graph, parent.Hash())
	sym := cfg.SymbolID(7)
	s.InterprocPrev.Facts[key] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Params:  []typ.Type{typ.String, typ.NewMap(typ.String, typ.Any)},
				Summary: []typ.Type{typ.String},
			},
		},
	}

	snapshot := s.GetInterprocFactsSnapshot(graph, parent)
	snapshotFact := snapshot.FunctionFacts[sym]
	snapshotFact.Params[1] = typ.Nil
	snapshot.FunctionFacts[sym] = api.FunctionFact{Summary: []typ.Type{typ.Number}}

	again := s.GetInterprocFactsSnapshot(graph, parent)
	if got := again.FunctionFacts.Params(sym)[1]; !typ.TypeEquals(got, typ.NewMap(typ.String, typ.Any)) {
		t.Fatalf("snapshot parameter evidence mutation leaked into store: %v", got)
	}
	if got := again.FunctionFacts.Summary(sym); len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("snapshot function fact mutation leaked into store: %v", got)
	}
}

func TestMergeInterprocFactsNext_ReconcilesDeltasWithinIteration(t *testing.T) {
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	sym := cfg.SymbolID(7)
	refined := typ.Func().Param("path", typ.String).Returns(typ.String).Build()
	broad := typ.Func().Param("path", typ.Any).Returns(typ.String).Build()

	s := NewSessionStore()
	first := api.Facts{FunctionFacts: api.FunctionFacts{sym: {Type: refined}}}
	s.MergeInterprocFactsNext(key, first)
	s.MergeInterprocFactsNext(key, api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Type: broad},
		},
	})

	got := s.InterprocNext.Facts[key].FunctionFacts.FunctionType(sym)
	if !typ.TypeEquals(got, refined) {
		t.Fatalf("expected update boundary to keep canonical refined function fact, got %v", got)
	}
}

func TestSnapshotInputs_RevalidateFactQueries(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	s := NewSessionStoreWithDB(database)
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	sym := cfg.SymbolID(7)

	calls := 0
	q := db.NewQuery("trackedFactsTest", func(ctx *db.QueryContext, key api.GraphKey) int {
		calls++
		facts, _ := s.snapshotInputs.factsFor(ctx, key)
		if len(facts.FunctionFacts.Summary(sym)) == 0 {
			return 0
		}
		return 1
	}, func(a, b int) bool { return a == b })

	if got := q.Get(ctx, key); got != 0 {
		t.Fatalf("initial query = %d, want 0", got)
	}
	if got := q.Get(ctx, key); got != 0 || calls != 1 {
		t.Fatalf("unchanged query = %d calls=%d, want 0/1", got, calls)
	}

	delta := api.Facts{FunctionFacts: api.FunctionFacts{
		sym: {Summary: []typ.Type{typ.String}},
	}}
	s.MergeInterprocFactsNext(key, delta)
	if got := q.Get(ctx, key); got != 1 || calls != 2 {
		t.Fatalf("changed query = %d calls=%d, want 1/2", got, calls)
	}

	s.MergeInterprocFactsNext(key, delta)
	if got := q.Get(ctx, key); got != 1 || calls != 2 {
		t.Fatalf("equal update query = %d calls=%d, want 1/2", got, calls)
	}
}

func TestSessionStore_Fields(t *testing.T) {
	s := &SessionStore{
		Module: &ModuleStore{
			Graphs: make(map[uint64]*cfg.Graph),
		},
	}
	if s.Module == nil {
		t.Error("Module should be set")
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

func TestIterationScratch_Fields(t *testing.T) {
	s := &IterationScratch{
		LiteralSigsByGraphID: make(map[uint64]map[*ast.FunctionExpr]*typ.Function),
	}
	if s.LiteralSigsByGraphID == nil {
		t.Error("LiteralSigsByGraphID should be initialized")
	}
}

func TestFixpointSwap_TracksChannelDiffsAndResetsNext(t *testing.T) {
	s := NewSessionStore()

	s.InterprocNext.Refinements[1] = &constraint.FunctionRefinement{Terminates: true}
	s.InterprocNext.Facts[api.GraphKey{GraphID: 7, ParentHash: 11}] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.String}},
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

	if s.Scratch == nil {
		t.Fatal("expected scratch to be initialized")
	}
	if s.InterprocPrev == nil || s.InterprocNext == nil {
		t.Fatal("expected interproc states to be initialized")
	}
	if s.Scratch.LiteralSigsByGraphID == nil {
		t.Fatal("expected scratch literal signatures map to be initialized")
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
