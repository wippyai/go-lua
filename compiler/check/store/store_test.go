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

func TestGetInterprocFacts_UsesStoredGraphParentHash(t *testing.T) {
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

	got := s.GetInterprocFacts(graph, currentParent)
	summary := got.FunctionFacts.Summary(cfg.SymbolID(1))
	if len(summary) != 1 || !typ.TypeEquals(summary[0], typ.String) {
		t.Fatalf("expected facts from stored parent hash, got %#v", summary)
	}
}

func TestGetInterprocFacts_OverlaysCurrentIterationFacts(t *testing.T) {
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

	got := s.GetInterprocFacts(graph, parent)
	summary := got.FunctionFacts.Summary(cfg.SymbolID(1))
	want := typ.NewUnion(typ.String, typ.Number)
	if len(summary) != 1 || !typ.TypeEquals(summary[0], want) {
		t.Fatalf("expected widened visible facts %v, got %#v", want, summary)
	}
}

func TestGetInterprocFacts_ReturnsImmutableFactContainers(t *testing.T) {
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

	facts := s.GetInterprocFacts(graph, parent)
	fact := facts.FunctionFacts[sym]
	fact.Params[1] = typ.Nil
	facts.FunctionFacts[sym] = api.FunctionFact{Summary: []typ.Type{typ.Number}}

	again := s.GetInterprocFacts(graph, parent)
	if got := again.FunctionFacts.Params(sym)[1]; !typ.TypeEquals(got, typ.NewMap(typ.String, typ.Any)) {
		t.Fatalf("fact parameter evidence mutation leaked into store: %v", got)
	}
	if got := again.FunctionFacts.Summary(sym); len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("function fact mutation leaked into store: %v", got)
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

func TestFactInputs_RevalidateFactQueries(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	s := NewSessionStoreWithDB(database)
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	sym := cfg.SymbolID(7)

	calls := 0
	q := db.NewQuery("trackedFactsTest", func(ctx *db.QueryContext, key api.GraphKey) int {
		calls++
		facts, _ := s.factInputs.factsFor(ctx, key)
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
		Evidence:      make(map[uint64]api.FlowEvidence),
	}
	if m.ModuleAliases[1] != "test" {
		t.Error("ModuleAliases not set correctly")
	}
}

func TestSessionStore_EvidenceForGraph(t *testing.T) {
	s := NewSessionStore()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}}},
		},
	}
	graph := cfg.Build(fn)
	evidence := s.EvidenceForGraph(graph)
	if len(evidence.ParameterUses) != 1 {
		t.Fatalf("expected parameter-use evidence, got %#v", evidence.ParameterUses)
	}

	overridden := api.FlowEvidence{
		NormalExit: api.NormalExitEvidence{Point: cfg.Point(99), Valid: true},
	}
	s.SetEvidenceForGraph(graph, overridden)
	if got := s.EvidenceForGraph(graph); got.NormalExit.Point != cfg.Point(99) || !got.NormalExit.Valid {
		t.Fatalf("expected cached override, got %#v", got.NormalExit)
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

func TestFixpointSwap_TracksChannelDiffsAndResetsNext(t *testing.T) {
	s := NewSessionStore()

	key := api.GraphKey{GraphID: 7, ParentHash: 11}
	s.InterprocNext.Facts[key] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {
				Summary:    []typ.Type{typ.String},
				Refinement: &constraint.FunctionRefinement{Terminates: true},
			},
		},
	}
	s.InterprocNext.Facts[api.ModuleFactsKey()] = api.Facts{
		ConstructorFields: api.ConstructorFields{
			3: {"v": typ.Number},
		},
	}

	if !s.FixpointSwap() {
		t.Fatal("expected fixpoint swap to report changes")
	}

	diffs := s.FixpointDiffs()
	if len(diffs) != 1 {
		t.Fatalf("expected one product diff, got %v", diffs)
	}
	if diffs[0] != "InterprocFacts" {
		t.Fatalf("unexpected diff order/content: %v", diffs)
	}

	if len(s.InterprocPrev.Facts) != 2 {
		t.Fatalf("expected prev facts populated, got %#v", s.InterprocPrev.Facts)
	}
	if len(s.InterprocNext.Facts) != 0 {
		t.Fatalf("expected next facts reset, got %#v", s.InterprocNext.Facts)
	}
	if s.InterprocPrev.Facts[key].FunctionFacts.Refinement(1) == nil {
		t.Fatalf("expected function refinement in product fact, got %#v", s.InterprocPrev.Facts[key])
	}
	if len(s.InterprocPrev.Facts[api.ModuleFactsKey()].ConstructorFields[3]) != 1 {
		t.Fatalf("expected constructor fields in module product fact, got %#v", s.InterprocPrev.Facts[api.ModuleFactsKey()])
	}
}

func TestClearInterprocState_InitializesMissingState(t *testing.T) {
	s := &SessionStore{}
	s.ClearInterprocState()

	if s.InterprocPrev == nil || s.InterprocNext == nil {
		t.Fatal("expected interproc states to be initialized")
	}
}

func TestFixpointDiffs_ReturnsCopy(t *testing.T) {
	s := NewSessionStore()
	key := registerFunctionForRefinementTest(t, s, 1)
	s.MergeInterprocFactsNext(key, api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Refinement: &constraint.FunctionRefinement{Terminates: true}},
		},
	})
	if !s.FixpointSwap() {
		t.Fatal("expected change from effect swap")
	}

	diffs := s.FixpointDiffs()
	if len(diffs) == 0 {
		t.Fatal("expected non-empty diffs")
	}
	diffs[0] = "MUTATED"

	diffs2 := s.FixpointDiffs()
	if len(diffs2) == 0 || diffs2[0] == "MUTATED" {
		t.Fatalf("expected defensive copy, got %v", diffs2)
	}
}

func registerFunctionForRefinementTest(t *testing.T, s *SessionStore, sym cfg.SymbolID) api.GraphKey {
	t.Helper()
	fn := &ast.FunctionExpr{}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("expected graph")
	}
	parent := scope.New()
	s.RegisterGraph(graph, fn)
	s.RegisterFunctionRef(sym, fn, graph, graph.ID(), 1)
	s.SetGraphParentHash(graph.ID(), parent.Hash())
	s.SetParentScope(parent.Hash(), parent)
	return api.KeyForGraph(graph, parent.Hash())
}
