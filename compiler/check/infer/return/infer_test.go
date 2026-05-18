package infer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestComputeFunctionFactsForGraph_Empty(t *testing.T) {
	inferencer := New(Config{})
	functionFacts, diags := inferencer.ComputeForGraph(RunContext{}, nil, nil)
	if functionFacts != nil {
		t.Error("nil graph should return nil function facts")
	}
	if len(diags) != 0 {
		t.Error("nil graph should return no diagnostics")
	}
}

func TestSeedReturnVectorsFromSeed_UsesKnownFunctionSymbolsOnly(t *testing.T) {
	localFuncs := map[cfg.SymbolID]*returns.LocalFuncInfo{
		1: nil,
		2: nil,
	}
	seed := map[cfg.SymbolID][]typ.Type{
		1: {typ.String},
		3: {typ.Number}, // not in local funcs; should be ignored
	}

	got := seedReturnVectorsFromSeed(localFuncs, seed)
	if len(got) != 1 {
		t.Fatalf("expected one seeded return vector, got %d", len(got))
	}
	if seeded := got[1]; len(seeded) != 1 || !typ.TypeEquals(seeded[0], typ.String) {
		t.Fatalf("unexpected seeded return vector for symbol 1: %v", seeded)
	}
	if _, ok := got[3]; ok {
		t.Fatalf("unexpected seed for unknown symbol 3: %v", got[3])
	}
}

func TestSeedReturnVectorsFromSeed_HandlesNilSeed(t *testing.T) {
	localFuncs := map[cfg.SymbolID]*returns.LocalFuncInfo{
		1: nil,
	}
	got := seedReturnVectorsFromSeed(localFuncs, nil)
	if got == nil {
		t.Fatal("expected non-nil return-vector map")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty return-vector map, got %v", got)
	}
}

func TestUniformFunctionScopes_UsesBaseForAllPoints(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	base := scope.New()

	scopes := uniformFunctionScopes(graph, base)
	if scopes == nil {
		t.Fatal("expected non-nil scopes")
	}

	graph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		if got := scopes[p]; got != base {
			t.Fatalf("expected base scope at point %d, got %p want %p", p, got, base)
		}
	})
	if got := scopes[graph.Entry()]; got != base {
		t.Fatalf("expected base scope at entry, got %p want %p", got, base)
	}
}

func TestExtractMapComponentType_Union(t *testing.T) {
	union := typ.NewUnion(
		typ.NewMap(typ.String, typ.Number),
		typ.NewRecord().MapComponent(typ.String, typ.Integer).Build(),
	)
	key, value, ok := extractMapComponentType(union)
	if !ok {
		t.Fatal("expected map component from union")
	}
	if !typ.TypeEquals(key, typ.String) {
		t.Fatalf("expected key type string, got %v", key)
	}
	if !typ.TypeEquals(value, typ.Number) {
		t.Fatalf("expected merged value type number, got %v", value)
	}
}

func TestReconcileSoftAnnotatedInference_PrefersAnnotatedMapOverEmptyRecord(t *testing.T) {
	base := typ.NewMap(typ.String, typ.Number)
	inferred := typ.NewRecord().Build()

	got := reconcileSoftAnnotatedInference(base, inferred)
	if !typ.TypeEquals(got, base) {
		t.Fatalf("expected base annotated map preserved, got %v", got)
	}
}

func TestReconcileSoftAnnotatedInference_RecordTemplateKeepsFields(t *testing.T) {
	base := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	inferred := typ.NewMap(typ.String, typ.Integer)

	got := reconcileSoftAnnotatedInference(base, inferred)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", got)
	}
	if !rec.HasMapComponent() {
		t.Fatal("expected map component on merged record")
	}
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("expected map key string, got %v", rec.MapKey)
	}
	if !typ.TypeEquals(rec.MapValue, typ.Number) {
		t.Fatalf("expected map value number, got %v", rec.MapValue)
	}
	foundName := false
	for _, f := range rec.Fields {
		if f.Name == "name" && typ.TypeEquals(f.Type, typ.String) {
			foundName = true
			break
		}
	}
	if !foundName {
		t.Fatalf("expected 'name: string' field preserved, got %+v", rec.Fields)
	}
}

func TestCollectAllReturnVectors_NormalizesAndFilters(t *testing.T) {
	inferencer := New(Config{})
	ctx := &returnInferenceContext{
		returnVectors: map[cfg.SymbolID][]typ.Type{
			0: {typ.String}, // invalid symbol id, ignored
			1: nil,          // empty return vector, ignored
			2: {nil, typ.String},
		},
	}

	got := inferencer.collectAllReturnVectors(ctx)
	if len(got) != 1 {
		t.Fatalf("expected one normalized return vector, got %d (%v)", len(got), got)
	}
	returnVector := got[2]
	if len(returnVector) != 2 {
		t.Fatalf("expected 2-slot return vector, got %v", returnVector)
	}
	if !typ.TypeEquals(returnVector[0], typ.Nil) {
		t.Fatalf("expected first slot normalized to nil, got %v", returnVector[0])
	}
	if !typ.TypeEquals(returnVector[1], typ.String) {
		t.Fatalf("expected second slot string, got %v", returnVector[1])
	}
}

func TestResolveLocalFunctionReturns_UsesCurrentVectorWithoutStore(t *testing.T) {
	inferencer := New(Config{})

	got := inferencer.resolveLocalFunctionReturns(nil, map[cfg.SymbolID][]typ.Type{
		1: {typ.String},
	}, 1)
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("expected string return vector, got %v", got)
	}

	unknownOnly := inferencer.resolveLocalFunctionReturns(nil, map[cfg.SymbolID][]typ.Type{
		1: {typ.Unknown},
	}, 1)
	if len(unknownOnly) != 1 || !typ.TypeEquals(unknownOnly[0], typ.Unknown) {
		t.Fatalf("expected unknown return vector without store recovery, got %v", unknownOnly)
	}

	if got := inferencer.resolveLocalFunctionReturns(nil, nil, 0); got != nil {
		t.Fatalf("expected nil return vector for symbol 0, got %v", got)
	}
}
