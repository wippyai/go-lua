package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestValueOriginDemandIndexedIteratorNestedField(t *testing.T) {
	entry := &ast.IdentExpr{Value: "entry"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{entry: cfg.SymbolID(12)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(11)] = 0

	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.WithPaths(
			constraint.NewPath(cfg.SymbolID(12), "entry"),
			constraint.NewPath(cfg.SymbolID(11), "tests"),
			flow.ValueOriginIndexedIterator,
			1,
		),
	}
	arg := &ast.AttrGetExpr{
		Object:    entry,
		Key:       &ast.StringExpr{Value: "id"},
		KeySyntax: ast.AttrKeyDot,
	}

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, arg, typ.String, demand)
	})
	want := typ.NewArray(typ.NewRecord().ReadonlyField("id", typ.String).Build())
	assertDemandType(t, got, 0, want)
}

func TestValueOriginDemandLiftsThroughNestedSourcePath(t *testing.T) {
	entry := &ast.IdentExpr{Value: "entry"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{entry: cfg.SymbolID(52)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(51)] = 0

	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.WithPaths(
			constraint.NewPath(cfg.SymbolID(52), "entry"),
			constraint.NewPath(cfg.SymbolID(51), "page").Field("tests"),
			flow.ValueOriginIndexedIterator,
			1,
		),
	}

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, &ast.AttrGetExpr{
			Object:    entry,
			Key:       &ast.StringExpr{Value: "id"},
			KeySyntax: ast.AttrKeyDot,
		}, typ.String, demand)
	})
	want := typ.NewRecord().
		ReadonlyField("tests", typ.NewArray(typ.NewRecord().ReadonlyField("id", typ.String).Build())).
		Build()
	assertDemandType(t, got, 0, want)
}

func TestValueOriginDemandUsesAllCoveringOrigins(t *testing.T) {
	entry := &ast.IdentExpr{Value: "entry"}
	entryMeta := &ast.AttrGetExpr{
		Object:    entry,
		Key:       &ast.StringExpr{Value: "meta"},
		KeySyntax: ast.AttrKeyDot,
	}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{entry: cfg.SymbolID(62)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(61)] = 0
	tr.paramBySym[cfg.SymbolID(63)] = 1

	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.
			WithPaths(
				constraint.NewPath(cfg.SymbolID(62), "entry"),
				constraint.NewPath(cfg.SymbolID(61), "tests"),
				flow.ValueOriginIndexedIterator,
				1,
			).
			WithPaths(
				constraint.NewPath(cfg.SymbolID(62), "entry").Field("meta"),
				constraint.NewPath(cfg.SymbolID(63), "metas"),
				flow.ValueOriginIndexedIterator,
				1,
			),
	}

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, &ast.AttrGetExpr{
			Object:    entryMeta,
			Key:       &ast.StringExpr{Value: "id"},
			KeySyntax: ast.AttrKeyDot,
		}, typ.String, demand)
	})
	assertDemandType(t, got, 0, typ.NewArray(typ.NewRecord().
		ReadonlyField("meta", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()))
	assertDemandType(t, got, 1, typ.NewArray(typ.NewRecord().ReadonlyField("id", typ.String).Build()))
}

func TestValueOriginDemandKeyedIteratorKeyAndValue(t *testing.T) {
	key := &ast.IdentExpr{Value: "name"}
	entry := &ast.IdentExpr{Value: "entry"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		key:   cfg.SymbolID(22),
		entry: cfg.SymbolID(23),
	})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(21)] = 0

	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.
			WithPaths(
				constraint.NewPath(cfg.SymbolID(22), "name"),
				constraint.NewPath(cfg.SymbolID(21), "items"),
				flow.ValueOriginKeyedIterator,
				0,
			).
			WithPaths(
				constraint.NewPath(cfg.SymbolID(23), "entry"),
				constraint.NewPath(cfg.SymbolID(21), "items"),
				flow.ValueOriginKeyedIterator,
				1,
			),
	}

	keyDemand := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, key, typ.String, demand)
	})
	assertDemandType(t, keyDemand, 0, typ.NewReadonlyMap(typ.String, typ.Any))

	valueDemand := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, &ast.AttrGetExpr{
			Object:    entry,
			Key:       &ast.StringExpr{Value: "id"},
			KeySyntax: ast.AttrKeyDot,
		}, typ.String, demand)
	})
	assertDemandType(t, valueDemand, 0, typ.NewReadonlyMap(typ.Any, typ.NewRecord().ReadonlyField("id", typ.String).Build()))
}

func TestValueOriginDemandDirectParamRoot(t *testing.T) {
	page := &ast.IdentExpr{Value: "page"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{page: cfg.SymbolID(31)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(31)] = 0

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&flow.PointState{}, page, typ.NewRecord().ReadonlyField("data_func", typ.String).Build(), demand)
	})
	assertDemandType(t, got, 0, typ.NewRecord().ReadonlyField("data_func", typ.String).Build())
}

func TestValueOriginDemandDirectParamRootUnderTruthyGuardAdmitsFalsyRoot(t *testing.T) {
	page := &ast.IdentExpr{Value: "page"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{page: cfg.SymbolID(31)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(31)] = 0
	path := constraint.NewPath(cfg.SymbolID(31), "page")
	out := flow.PointState{Cond: constraint.FromConstraints(constraint.Truthy{Path: path})}

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, page, typ.String, demand)
	})
	assertDemandType(t, got, 0, typ.NewUnion(typ.String, typ.Nil, typ.False))
}

func TestValueOriginDemandDirectParamFieldPath(t *testing.T) {
	entry := &ast.IdentExpr{Value: "entry"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{entry: cfg.SymbolID(41)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(41)] = 0

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&flow.PointState{}, &ast.AttrGetExpr{
			Object:    entry,
			Key:       &ast.StringExpr{Value: "id"},
			KeySyntax: ast.AttrKeyDot,
		}, typ.String, demand)
	})
	assertDemandType(t, got, 0, typ.NewRecord().ReadonlyField("id", typ.String).Build())
}

func TestValueOriginDemandDirectParamFieldPathUnderTruthyGuardAdmitsFalsyLeaf(t *testing.T) {
	entry := &ast.IdentExpr{Value: "entry"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{entry: cfg.SymbolID(41)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(41)] = 0
	path := constraint.NewPath(cfg.SymbolID(41), "entry").Field("id")
	out := flow.PointState{Cond: constraint.FromConstraints(constraint.Truthy{Path: path})}

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, &ast.AttrGetExpr{
			Object:    entry,
			Key:       &ast.StringExpr{Value: "id"},
			KeySyntax: ast.AttrKeyDot,
		}, typ.String, demand)
	})
	assertDemandType(t, got, 0, typ.NewRecord().
		ReadonlyField("id", typ.NewUnion(typ.String, typ.Nil, typ.False)).
		Build())
}

func TestValueOriginInvalidatedByGenericWriteEffect(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	entryPath := constraint.NewPath(cfg.SymbolID(71), "entry")
	sourcePath := constraint.NewPath(cfg.SymbolID(72), "tests")
	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.WithPaths(entryPath, sourcePath, flow.ValueOriginIndexedIterator, 1),
	}

	tr.applyWriteEffect(&out, WriteEffect{
		Place:        Place{Root: cfg.SymbolID(71), Steps: []PlaceStep{{Kind: PlaceStepStaticMember, Member: value.MemberField("id")}}},
		Value:        product.FromType(typ.String),
		RecordStatic: true,
	})

	if len(out.ValueOrigins.Entries()) != 0 {
		t.Fatalf("generic WriteEffect kept stale ValueOrigin: %s", out.ValueOrigins.Format())
	}
}

func valueOriginInput(t *testing.T, symbols map[*ast.IdentExpr]cfg.SymbolID) input.Inputs {
	t.Helper()
	in := input.BuildFromFunction(&ast.FunctionExpr{ParList: &ast.ParList{}}, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	for ident, sym := range symbols {
		in.Graph.Bindings().Bind(ident, sym)
		in.Graph.Bindings().SetName(sym, ident.Value)
	}
	return in
}

func collectDemand(t *testing.T, run func(func(int, paramevidence.ParamContract))) paramevidence.Contracts {
	t.Helper()
	out := paramevidence.Contracts{}
	run(func(idx int, contract paramevidence.ParamContract) {
		out = paramevidence.JoinDemand(out, idx, contract)
	})
	return out
}

func assertDemandType(t *testing.T, contracts paramevidence.Contracts, idx int, want typ.Type) {
	t.Helper()
	got, ok := contracts[idx]
	if !ok {
		t.Fatalf("missing demand for param %d", idx)
	}
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("demand[%d] = %v, want %v", idx, got.ProjectValue(), want)
	}
}
