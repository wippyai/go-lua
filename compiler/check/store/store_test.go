package store

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPostflowProjectionStateInitialization(t *testing.T) {
	state := newPostflowProjectionState()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.facts == nil {
		t.Error("Facts map should be initialized")
	}
}

func TestFunctionFactsSummaryProjection(t *testing.T) {
	facts := interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})},
			},
		},
	}
	got := functionfact.FactsProjection(facts.FunctionFacts).ReturnSummary(cfg.SymbolID(1))
	if len(got) != 1 || got[0] != typ.String {
		t.Fatalf("unexpected summary: %#v", got)
	}
}

func TestFunctionFactsNarrowProjection(t *testing.T) {
	facts := interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(2): {
				Returns: api.FunctionReturnProjection{Postflow: product.LiftVector([]typ.Type{typ.Number})},
			},
		},
	}
	got := functionfact.FactsProjection(facts.FunctionFacts).NarrowSummary(cfg.SymbolID(2))
	if len(got) != 1 || got[0] != typ.Number {
		t.Fatalf("unexpected narrow summary: %#v", got)
	}
}

func TestFunctionFactsSiblingProjection(t *testing.T) {
	fn := typ.Func().Returns(typ.Boolean).Build()
	facts := interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(3): {
				Public: api.FunctionPublicProjection{Signature: fn},
			},
		},
	}
	got := functionfact.FactsProjection(facts.FunctionFacts).Type(cfg.SymbolID(3), functionfact.ProjectionSibling, api.SynthModeDeclared)
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("unexpected function type: %#v", got)
	}
}

func TestPostflowProjectionUsesStoredGraphParentHash(t *testing.T) {
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
	s.postflowPrev.facts[key] = interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	}

	functionFacts := s.FunctionFactsProjectionForExport(graph, currentParent)
	summary := functionfact.FactsProjection(functionFacts).ReturnSummary(cfg.SymbolID(1))
	if len(summary) != 1 || !typ.TypeEquals(summary[0], typ.String) {
		t.Fatalf("expected facts from stored parent hash, got %#v", summary)
	}
}

func TestPostflowProjectionOverlaysCurrentIterationFacts(t *testing.T) {
	graph := cfg.Build(&ast.FunctionExpr{})
	if graph == nil || graph.ID() == 0 {
		t.Fatal("expected graph with stable ID")
	}

	parent := scope.New().WithType("T", typ.String)
	s := NewSessionStore()
	s.SetGraphParentHash(graph.ID(), parent.Hash())
	s.SetParentScope(parent.Hash(), parent)
	key := api.KeyForGraph(graph, parent.Hash())
	s.postflowPrev.facts[key] = interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	}
	s.postflowNext.facts[key] = interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			cfg.SymbolID(1): {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
		},
	}

	functionFacts := s.FunctionFactsProjectionForExport(graph, parent)
	summary := functionfact.FactsProjection(functionFacts).ReturnSummary(cfg.SymbolID(1))
	want := typ.NewUnion(typ.String, typ.Number)
	if len(summary) != 1 || !typ.TypeEquals(summary[0], want) {
		t.Fatalf("expected widened visible facts %v, got %#v", want, summary)
	}
}

func TestPostflowProjectionReturnsImmutableFactContainers(t *testing.T) {
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
	s.postflowPrev.facts[key] = interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Call:    api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.String, typ.NewMap(typ.String, typ.Any)})},
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})},
			},
		},
	}

	facts := s.FunctionFactsProjectionForExport(graph, parent)
	fact := facts[sym]
	fact.Call.Params[1] = product.FromType(typ.Nil)
	facts[sym] = api.FunctionFact{Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}}

	again := s.FunctionFactsProjectionForExport(graph, parent)
	if got := functionfact.FactsProjection(again).PublicParameterEvidence(sym)[1]; !typ.TypeEquals(got, typ.NewMap(typ.String, typ.Any)) {
		t.Fatalf("fact parameter evidence mutation leaked into store: %v", got)
	}
	if got := functionfact.FactsProjection(again).ReturnSummary(sym); len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("function fact mutation leaked into store: %v", got)
	}
}

func TestPostflowProjectionDeltasReconcileWithinIteration(t *testing.T) {
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	sym := cfg.SymbolID(7)
	refined := typ.Func().Param("path", typ.String).Returns(typ.String).Build()
	broad := typ.Func().Param("path", typ.Any).Returns(typ.String).Build()

	s := NewSessionStore()
	first := interproc.ProjectionProduct{FunctionFacts: api.FunctionFacts{sym: {Public: api.FunctionPublicProjection{Signature: refined}}}}
	s.mergePostflowProjectionProductNext(key, first)
	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			sym: {Public: api.FunctionPublicProjection{Signature: broad}},
		},
	})

	got := functionfact.FactsProjection(s.postflowNext.facts[key].FunctionFacts).Type(sym, functionfact.ProjectionSibling, api.SynthModeDeclared)
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
		functionFacts, _ := s.factInputs.functionFactsFor(ctx, key)
		if len(functionfact.FactsProjection(functionFacts).ReturnSummary(sym)) == 0 {
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

	delta := interproc.ProjectionProduct{FunctionFacts: api.FunctionFacts{
		sym: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
	}}
	s.mergePostflowProjectionProductNext(key, delta)
	if got := q.Get(ctx, key); got != 0 || calls != 1 {
		t.Fatalf("same-iteration write query = %d calls=%d, want stable 0/1 before swap", got, calls)
	}
	s.AdvancePostflowProjections()
	if got := q.Get(ctx, key); got != 1 || calls != 2 {
		t.Fatalf("changed query = %d calls=%d, want 1/2", got, calls)
	}

	s.mergePostflowProjectionProductNext(key, delta)
	s.AdvancePostflowProjections()
	if got := q.Get(ctx, key); got != 1 || calls != 2 {
		t.Fatalf("equal update query = %d calls=%d, want 1/2", got, calls)
	}
}

func TestFactInputs_FunctionFactProjectionTracksOneSymbol(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	s := NewSessionStoreWithDB(database)
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	sym7 := cfg.SymbolID(7)
	sym8 := cfg.SymbolID(8)

	calls := 0
	q := db.NewQuery("trackedFunctionFactProjectionTest", func(ctx *db.QueryContext, key api.FunctionFactKey) int {
		calls++
		ff, ok := s.factInputs.functionFactFor(ctx, key)
		if !ok {
			return 0
		}
		return len(ff.Returns.Preflow)
	}, func(a, b int) bool { return a == b })
	queryKey := api.FunctionFactKey{GraphKey: key, Symbol: sym7}

	if got := q.Get(ctx, queryKey); got != 0 || calls != 1 {
		t.Fatalf("initial projection = %d calls=%d, want 0/1", got, calls)
	}

	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{FunctionFacts: api.FunctionFacts{
		sym7: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		sym8: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
	}})
	s.AdvancePostflowProjections()
	if got := q.Get(ctx, queryKey); got != 1 || calls != 2 {
		t.Fatalf("changed projection = %d calls=%d, want 1/2", got, calls)
	}

	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{FunctionFacts: api.FunctionFacts{
		sym8: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number, typ.String})}},
	}})
	s.AdvancePostflowProjections()
	if got := q.Get(ctx, queryKey); got != 1 || calls != 2 {
		t.Fatalf("unrelated symbol changed projection = %d calls=%d, want cached 1/2", got, calls)
	}

	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{FunctionFacts: api.FunctionFacts{
		sym7: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String, typ.Number})}},
	}})
	s.AdvancePostflowProjections()
	if got := q.Get(ctx, queryKey); got != 2 || calls != 3 {
		t.Fatalf("tracked symbol changed projection = %d calls=%d, want 2/3", got, calls)
	}
}

func TestFactInputs_CapturedTypesUseRecursiveFactEquality(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	s := NewSessionStoreWithDB(database)
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	sym := cfg.SymbolID(7)
	left := typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	right := typ.NewRecursive("Builder", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})

	calls := 0
	q := db.NewQuery("trackedCapturedTypeProjectionTest", func(ctx *db.QueryContext, key api.CapturedTypeKey) int {
		calls++
		t, ok := s.factInputs.capturedTypeFor(ctx, key)
		if !ok || t == nil {
			return 0
		}
		return len(t.String())
	}, func(a, b int) bool { return a == b })
	queryKey := api.CapturedTypeKey{GraphKey: key, Symbol: sym}

	if got := q.Get(ctx, queryKey); got != 0 || calls != 1 {
		t.Fatalf("initial projection = %d calls=%d, want 0/1", got, calls)
	}
	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{CapturedTypes: api.CapturedTypes{sym: product.FromType(left)}})
	s.AdvancePostflowProjections()
	if got := q.Get(ctx, queryKey); got == 0 || calls != 2 {
		t.Fatalf("changed projection = %d calls=%d, want nonzero/2", got, calls)
	}
	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{CapturedTypes: api.CapturedTypes{sym: product.FromType(right)}})
	s.AdvancePostflowProjections()
	if got := q.Get(ctx, queryKey); got == 0 || calls != 2 {
		t.Fatalf("equivalent recursive projection = %d calls=%d, want cached nonzero/2", got, calls)
	}
}

func TestFactInputs_PublishOnlyAtFixpointBoundary(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	s := NewSessionStoreWithDB(database)
	key := api.GraphKey{GraphID: 1, ParentHash: 2}

	calls := 0
	q := db.NewQuery("batchedTrackedFactsTest", func(ctx *db.QueryContext, key api.GraphKey) int {
		calls++
		functionFacts, _ := s.factInputs.functionFactsFor(ctx, key)
		return len(functionFacts)
	}, func(a, b int) bool { return a == b })

	if got := q.Get(ctx, key); got != 0 || calls != 1 {
		t.Fatalf("initial query = %d calls=%d, want 0/1", got, calls)
	}

	beforeWrites := database.Revision()
	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{FunctionFacts: api.FunctionFacts{
		cfg.SymbolID(7): {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
	}})
	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{FunctionFacts: api.FunctionFacts{
		cfg.SymbolID(8): {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
	}})
	if got := database.Revision(); got != beforeWrites {
		t.Fatalf("next-product writes bumped query revision: got %d want %d", got, beforeWrites)
	}
	if got := q.Get(ctx, key); got != 0 || calls != 1 {
		t.Fatalf("pre-swap query = %d calls=%d, want stable 0/1", got, calls)
	}

	s.AdvancePostflowProjections()
	if got := database.Revision(); got != beforeWrites+1 {
		t.Fatalf("fixpoint swap bumped revision %d times, got %d want %d", got-beforeWrites, got, beforeWrites+1)
	}
	if got := q.Get(ctx, key); got != 2 || calls != 2 {
		t.Fatalf("synced query = %d calls=%d, want 2/2", got, calls)
	}

	afterSync := database.Revision()
	s.AdvancePostflowProjections()
	if got := database.Revision(); got != afterSync {
		t.Fatalf("clean swap bumped revision: got %d want %d", got, afterSync)
	}
	if got := q.Get(ctx, key); got != 2 || calls != 2 {
		t.Fatalf("clean query = %d calls=%d, want 2/2", got, calls)
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

func TestAdvancePostflowProjections_TracksChannelDiffsAndResetsNext(t *testing.T) {
	s := NewSessionStore()

	key := api.GraphKey{GraphID: 7, ParentHash: 11}
	s.postflowNext.facts[key] = interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			1: {
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})},
				Effects: api.FunctionEffectProjection{Refinement: &constraint.FunctionRefinement{Terminates: true}},
			},
		},
	}
	s.postflowNext.facts[api.ModuleFactsKey()] = interproc.ProjectionProduct{
		ConstructorFields: api.ConstructorFields{
			3: {constraint.Segment{Kind: constraint.SegmentField, Name: "v"}: product.FromType(typ.Number)},
		},
	}

	if !s.AdvancePostflowProjections() {
		t.Fatal("expected fixpoint swap to report changes")
	}

	diffs := s.PostflowProjectionDiffs()
	if len(diffs) != 1 {
		t.Fatalf("expected one product diff, got %v", diffs)
	}
	if diffs[0] != "PostflowProjection" {
		t.Fatalf("unexpected diff order/content: %v", diffs)
	}

	if len(s.postflowPrev.facts) != 2 {
		t.Fatalf("expected prev facts populated, got %#v", s.postflowPrev.facts)
	}
	if len(s.postflowNext.facts) != 0 {
		t.Fatalf("expected next facts reset, got %#v", s.postflowNext.facts)
	}
	if functionfact.FactsProjection(s.postflowPrev.facts[key].FunctionFacts).Refinement(1) == nil {
		t.Fatalf("expected function refinement in product fact, got %#v", s.postflowPrev.facts[key])
	}
	if len(s.postflowPrev.facts[api.ModuleFactsKey()].ConstructorFields[3]) != 1 {
		t.Fatalf("expected constructor fields in module product fact, got %#v", s.postflowPrev.facts[api.ModuleFactsKey()])
	}
}

func TestClearPostflowProjectionState_InitializesMissingState(t *testing.T) {
	s := &SessionStore{}
	s.ClearPostflowProjectionState()

	if s.postflowPrev == nil || s.postflowNext == nil {
		t.Fatal("expected interproc states to be initialized")
	}
}

func TestPostflowProjectionDiffsReturnCopy(t *testing.T) {
	s := NewSessionStore()
	key := registerFunctionForRefinementTest(t, s, 1)
	s.mergePostflowProjectionProductNext(key, interproc.ProjectionProduct{
		FunctionFacts: api.FunctionFacts{
			1: {Effects: api.FunctionEffectProjection{Refinement: &constraint.FunctionRefinement{Terminates: true}}},
		},
	})
	if !s.AdvancePostflowProjections() {
		t.Fatal("expected change from effect swap")
	}

	diffs := s.PostflowProjectionDiffs()
	if len(diffs) == 0 {
		t.Fatal("expected non-empty diffs")
	}
	diffs[0] = "MUTATED"

	diffs2 := s.PostflowProjectionDiffs()
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
