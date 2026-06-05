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
		ValueOrigins: flow.ValueOriginFacts{}.WithAddresses(
			testFlowPathAddress(t, constraint.NewPath(cfg.SymbolID(12), "entry")),
			testFlowPathAddress(t, constraint.NewPath(cfg.SymbolID(11), "tests")),
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

func TestValueOriginDemandRoutesThroughAppendElementFieldOrigin(t *testing.T) {
	routeEntry := &ast.IdentExpr{Value: "route_entry"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{routeEntry: cfg.SymbolID(156)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(120)] = 0

	routePath := constraint.NewPath(cfg.SymbolID(155), "route")
	routeEntryPath := constraint.NewPath(cfg.SymbolID(156), "route_entry")
	pendingRoutes := constraint.NewPath(cfg.SymbolID(122), "graph").Field("pending_routes")
	opPath := constraint.NewPath(cfg.SymbolID(154), "op")
	operationsPath := constraint.NewPath(cfg.SymbolID(120), "operations")
	out := flow.PointState{
		PathAliases: flow.PathAliasFacts{}.WithAddresses(testFlowPathAddress(t, routeEntryPath), testFlowPathAddress(t, routePath)),
		ValueOrigins: flow.ValueOriginFacts{}.
			WithAddresses(testFlowPathAddress(t, routePath), testFlowPathAddress(t, pendingRoutes), flow.ValueOriginIndexedIterator, 1).
			WithAddresses(testFlowPathAddress(t, opPath), testFlowPathAddress(t, operationsPath), flow.ValueOriginIndexedIterator, 1),
		KeyPresence: flow.KeyPresenceFacts{}.
			WithAppendHistoryBasePath(pendingRoutes).
			WithAppendElementFieldOriginPaths(
				pendingRoutes,
				[]constraint.Segment{{Kind: constraint.SegmentField, Name: "target_name"}},
				opPath.Field("config").Field("target"),
			),
	}

	arg := &ast.AttrGetExpr{
		Object:    routeEntry,
		Key:       &ast.StringExpr{Value: "target_name"},
		KeySyntax: ast.AttrKeyDot,
	}
	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, arg, typ.String, demand)
	})
	want := typ.NewArray(typ.NewRecord().ReadonlyField("config", typ.NewRecord().
		ReadonlyField("target", typ.String).
		Build()).Build())
	assertDemandType(t, got, 0, want)
}

func TestAssignmentProvenanceUsesCFGSourceSymbolFallback(t *testing.T) {
	route := &ast.IdentExpr{Value: "route"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{})
	tr := New(in, Config{})
	target := cfg.AssignTarget{Kind: cfg.TargetIdent, Name: "route_entry", Symbol: cfg.SymbolID(156)}
	srcSym := cfg.SymbolID(155)

	provenance, ok := tr.assignmentProvenanceEffectWithSourceSymbol(target, route, srcSym, product.AbstractValue{})
	if !ok {
		t.Fatal("assignment provenance did not use CFG source symbol fallback")
	}
	if provenance.SourcePath.Symbol != srcSym {
		t.Fatalf("source symbol = %d, want %d", provenance.SourcePath.Symbol, srcSym)
	}

	out := flow.PointState{}
	tr.applyAssignmentProvenanceEffect(&out, provenance)
	if aliases := out.PathAliases.AliasesOfAddress(testFlowPathAddress(t, constraint.NewPath(cfg.SymbolID(156), "route_entry"))); len(aliases) != 1 ||
		aliases[0].Source != flow.KeyPresencePathKey(constraint.NewPath(srcSym, "route")) {
		t.Fatalf("path aliases = %s, want route_entry<-route", out.PathAliases.Format())
	}
}

func TestConditionReadDemandUnaryLengthOperand(t *testing.T) {
	operations := &ast.IdentExpr{Value: "operations"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{operations: cfg.SymbolID(12)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(12)] = 0

	cond := &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      &ast.UnaryLenOpExpr{Expr: operations},
		Rhs:      &ast.NumberExpr{Value: "0"},
	}
	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandConditionReads(&flow.PointState{}, cond, demand)
	})
	assertDemandType(t, got, 0, lengthContextType())
}

func TestCapabilityDemandDirectParamFieldPathUnderTruthyGuardAdmitsFalsyLeaf(t *testing.T) {
	template := &ast.IdentExpr{Value: "template"}
	operations := &ast.AttrGetExpr{
		Object:    template,
		Key:       &ast.StringExpr{Value: "operations"},
		KeySyntax: ast.AttrKeyDot,
	}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{template: cfg.SymbolID(12)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(12)] = 0
	path := constraint.NewPath(cfg.SymbolID(12), "template").Field("operations")
	out := flow.PointState{Cond: constraint.FromConstraints(constraint.Truthy{Path: path})}

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCapabilityCtx(&out, operations, paramevidence.CapabilityLength, demand)
	})
	assertDemandType(t, got, 0, typ.NewRecord().
		ReadonlyField("operations", typ.NewUnion(lengthContextType(), typ.Nil, typ.False)).
		Build())
}

func TestValueOriginDemandLiftsThroughNestedSourcePath(t *testing.T) {
	entry := &ast.IdentExpr{Value: "entry"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{entry: cfg.SymbolID(52)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(51)] = 0

	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.WithAddresses(
			testFlowPathAddress(t, constraint.NewPath(cfg.SymbolID(52), "entry")),
			testFlowPathAddress(t, constraint.NewPath(cfg.SymbolID(51), "page").Field("tests")),
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

	entryPath := constraint.NewPath(cfg.SymbolID(62), "entry")
	testsPath := constraint.NewPath(cfg.SymbolID(61), "tests")
	metasPath := constraint.NewPath(cfg.SymbolID(63), "metas")
	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.
			WithAddresses(
				testFlowPathAddress(t, entryPath),
				testFlowPathAddress(t, testsPath),
				flow.ValueOriginIndexedIterator,
				1,
			).
			WithAddresses(
				testFlowPathAddress(t, entryPath.Field("meta")),
				testFlowPathAddress(t, metasPath),
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

	namePath := constraint.NewPath(cfg.SymbolID(22), "name")
	itemsPath := constraint.NewPath(cfg.SymbolID(21), "items")
	entryPath := constraint.NewPath(cfg.SymbolID(23), "entry")
	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.
			WithAddresses(
				testFlowPathAddress(t, namePath),
				testFlowPathAddress(t, itemsPath),
				flow.ValueOriginKeyedIterator,
				0,
			).
			WithAddresses(
				testFlowPathAddress(t, entryPath),
				testFlowPathAddress(t, itemsPath),
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

func TestValueOriginContractDemandGuardedDerivedLeafAdmitsFalsyLeaf(t *testing.T) {
	op := &ast.IdentExpr{Value: "op"}
	opTemplate := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object:    op,
			Key:       &ast.StringExpr{Value: "config"},
			KeySyntax: ast.AttrKeyDot,
		},
		Key:       &ast.StringExpr{Value: "template"},
		KeySyntax: ast.AttrKeyDot,
	}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{op: cfg.SymbolID(52)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(51)] = 0
	sourcePath := constraint.NewPath(cfg.SymbolID(51), "template").Field("operations")
	valuePath := constraint.NewPath(cfg.SymbolID(52), "op").Field("config").Field("template")
	out := flow.PointState{
		Cond: constraint.FromConstraints(constraint.Truthy{Path: valuePath}),
		ValueOrigins: flow.ValueOriginFacts{}.WithAddresses(
			testFlowPathAddress(t, constraint.NewPath(cfg.SymbolID(52), "op")),
			testFlowPathAddress(t, sourcePath),
			flow.ValueOriginIndexedIterator,
			1,
		),
	}
	calleeTemplateContract := paramevidence.DemandFromPathCapability(
		[]constraint.Segment{{Kind: constraint.SegmentField, Name: "operations"}},
		paramevidence.CapabilityLength,
	)

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprContractCtx(&out, opTemplate, calleeTemplateContract, demand)
	})
	wantElement := typ.NewRecord().ReadonlyField("config", typ.NewRecord().
		ReadonlyField("template", typ.NewUnion(
			typ.NewRecord().ReadonlyField("operations", lengthContextType()).Build(),
			typ.Nil,
			typ.False,
		)).
		Build()).Build()
	assertDemandType(t, got, 0, typ.NewRecord().
		ReadonlyField("operations", typ.NewArray(wantElement)).
		Build())
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

func TestValueOriginDemandDirectParamFieldPathUnderNonEmptyGuardAdmitsGuardedAwayLeaf(t *testing.T) {
	page := &ast.IdentExpr{Value: "page"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{page: cfg.SymbolID(42)})
	tr := New(in, Config{})
	tr.paramBySym[cfg.SymbolID(42)] = 0
	pagePath := constraint.NewPath(cfg.SymbolID(42), "page")
	out := flow.PointState{Cond: constraint.FromConstraints(
		constraint.Truthy{Path: pagePath.Field("data_func")},
		constraint.FieldNotEquals{
			Target: pagePath,
			Field:  "data_func",
			Value:  typ.LiteralString(""),
		},
	)}

	got := collectDemand(t, func(demand func(int, paramevidence.ParamContract)) {
		tr.demandExprCtx(&out, &ast.AttrGetExpr{
			Object:    page,
			Key:       &ast.StringExpr{Value: "data_func"},
			KeySyntax: ast.AttrKeyDot,
		}, typ.String, demand)
	})
	assertDemandType(t, got, 0, typ.NewRecord().
		ReadonlyField("data_func", typ.NewUnion(typ.String, typ.Nil, typ.False, typ.LiteralString(""))).
		Build())
}

func TestValueOriginInvalidatedByGenericWriteEffect(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	entryPath := constraint.NewPath(cfg.SymbolID(71), "entry")
	sourcePath := constraint.NewPath(cfg.SymbolID(72), "tests")
	out := flow.PointState{
		ValueOrigins: flow.ValueOriginFacts{}.WithAddresses(
			testFlowPathAddress(t, entryPath),
			testFlowPathAddress(t, sourcePath),
			flow.ValueOriginIndexedIterator,
			1,
		),
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
