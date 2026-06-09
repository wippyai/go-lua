package store

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFunctionFactsSummaryProjection(t *testing.T) {
	facts := api.FunctionFacts{
		cfg.SymbolID(1): {
			Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})},
		},
	}
	got := functionfact.FactsProjection(facts).ReturnSummary(cfg.SymbolID(1))
	if len(got) != 1 || got[0] != typ.String {
		t.Fatalf("unexpected summary: %#v", got)
	}
}

func TestFunctionFactsNarrowProjection(t *testing.T) {
	facts := api.FunctionFacts{
		cfg.SymbolID(2): {
			Returns: api.FunctionReturnProjection{Narrow: product.LiftVector([]typ.Type{typ.Number})},
		},
	}
	got := functionfact.FactsProjection(facts).NarrowSummary(cfg.SymbolID(2))
	if len(got) != 1 || got[0] != typ.Number {
		t.Fatalf("unexpected narrow summary: %#v", got)
	}
}

func TestFunctionFactsSiblingProjection(t *testing.T) {
	fn := typ.Func().Returns(typ.Boolean).Build()
	facts := api.FunctionFacts{
		cfg.SymbolID(3): {
			Public: api.FunctionPublicProjection{Signature: fn},
		},
	}
	got := functionfact.FactsProjection(facts).Type(cfg.SymbolID(3), functionfact.ProjectionSibling, api.SynthModeDeclared)
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("unexpected function type: %#v", got)
	}
}

func TestCanonicalFunctionFactsProjectionUsesStoredGraphParentHash(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	graph := cfg.Build(&ast.FunctionExpr{})
	if graph == nil || graph.ID() == 0 {
		t.Fatal("expected graph with stable ID")
	}

	storedParent := scope.New().WithType("T", typ.String)
	currentParent := scope.New().WithType("T", typ.Number)
	if storedParent.Hash() == currentParent.Hash() {
		t.Fatal("test requires different parent hashes")
	}

	s := NewSessionStoreWithDB(database)
	restore := s.PushFactReadContext(ctx)
	defer restore()

	s.SetGraphParentHash(graph.ID(), storedParent.Hash())
	s.SetParentScope(storedParent.Hash(), storedParent)
	key := api.KeyForGraph(graph, storedParent.Hash())
	s.SetCanonicalFunctionFactsProjection(map[api.GraphKey]api.FunctionFacts{
		key: {
			cfg.SymbolID(1): {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
		},
	})

	functionFacts := s.CanonicalFunctionFactsProjectionForExport(graph, currentParent)
	summary := functionfact.FactsProjection(functionFacts).ReturnSummary(cfg.SymbolID(1))
	if len(summary) != 1 || !typ.TypeEquals(summary[0], typ.String) {
		t.Fatalf("expected facts from stored parent hash, got %#v", summary)
	}
}

func TestCanonicalFunctionFactsProjectionReturnsImmutableFactContainers(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	graph := cfg.Build(&ast.FunctionExpr{})
	if graph == nil || graph.ID() == 0 {
		t.Fatal("expected graph with stable ID")
	}

	parent := scope.New().WithType("T", typ.String)
	s := NewSessionStoreWithDB(database)
	restore := s.PushFactReadContext(ctx)
	defer restore()

	s.SetGraphParentHash(graph.ID(), parent.Hash())
	s.SetParentScope(parent.Hash(), parent)
	key := api.KeyForGraph(graph, parent.Hash())
	sym := cfg.SymbolID(7)
	s.SetCanonicalFunctionFactsProjection(map[api.GraphKey]api.FunctionFacts{
		key: {
			sym: {
				Call:    api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.String, typ.NewMap(typ.String, typ.Any)})},
				Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})},
			},
		},
	})

	facts := s.CanonicalFunctionFactsProjectionForExport(graph, parent)
	fact := facts[sym]
	fact.Call.Params[1] = product.FromType(typ.Nil)
	facts[sym] = api.FunctionFact{Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}}

	again := s.CanonicalFunctionFactsProjectionForExport(graph, parent)
	if got := functionfact.FactsProjection(again).PublicParameterEvidence(sym)[1]; !typ.TypeEquals(got, typ.NewMap(typ.String, typ.Any)) {
		t.Fatalf("fact parameter evidence mutation leaked into store: %v", got)
	}
	if got := functionfact.FactsProjection(again).ReturnSummary(sym); len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("function fact mutation leaked into store: %v", got)
	}
}

func TestFactInputs_RevalidateCanonicalProjectionQueries(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)
	s := NewSessionStoreWithDB(database)
	key := api.GraphKey{GraphID: 1, ParentHash: 2}
	sym := cfg.SymbolID(7)

	calls := 0
	q := db.NewQuery("trackedCanonicalFactsTest", func(ctx *db.QueryContext, key api.GraphKey) int {
		calls++
		functionFacts, _ := s.factInputs.canonicalFunctionFactsFor(ctx, key)
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

	fact := api.FunctionFact{Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}}
	s.SetCanonicalFunctionFactsProjection(map[api.GraphKey]api.FunctionFacts{key: {sym: fact}})
	if got := q.Get(ctx, key); got != 1 || calls != 2 {
		t.Fatalf("changed query = %d calls=%d, want 1/2", got, calls)
	}

	s.SetCanonicalFunctionFactsProjection(map[api.GraphKey]api.FunctionFacts{key: {sym: fact}})
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
	q := db.NewQuery("trackedCanonicalFunctionFactProjectionTest", func(ctx *db.QueryContext, key api.FunctionFactKey) int {
		calls++
		ff, ok := s.factInputs.canonicalFunctionFactFor(ctx, key)
		if !ok {
			return 0
		}
		return len(ff.Returns.Preflow)
	}, func(a, b int) bool { return a == b })
	queryKey := api.FunctionFactKey{GraphKey: key, Symbol: sym7}

	if got := q.Get(ctx, queryKey); got != 0 || calls != 1 {
		t.Fatalf("initial projection = %d calls=%d, want 0/1", got, calls)
	}

	s.SetCanonicalFunctionFactsProjection(map[api.GraphKey]api.FunctionFacts{
		key: {
			sym7: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
			sym8: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number})}},
		},
	})
	if got := q.Get(ctx, queryKey); got != 1 || calls != 2 {
		t.Fatalf("changed projection = %d calls=%d, want 1/2", got, calls)
	}

	s.SetCanonicalFunctionFactsProjection(map[api.GraphKey]api.FunctionFacts{
		key: {
			sym7: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String})}},
			sym8: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number, typ.String})}},
		},
	})
	if got := q.Get(ctx, queryKey); got != 1 || calls != 2 {
		t.Fatalf("unrelated symbol changed projection = %d calls=%d, want cached 1/2", got, calls)
	}

	s.SetCanonicalFunctionFactsProjection(map[api.GraphKey]api.FunctionFacts{
		key: {
			sym7: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String, typ.Number})}},
			sym8: {Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.Number, typ.String})}},
		},
	})
	if got := q.Get(ctx, queryKey); got != 2 || calls != 3 {
		t.Fatalf("tracked symbol changed projection = %d calls=%d, want 2/3", got, calls)
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
