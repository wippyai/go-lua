package summary

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func testSummaryStableAddress(t *testing.T, path constraint.Path) flow.StableAddress {
	t.Helper()
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		t.Fatalf("stable address for path %s", path.Key())
	}
	return addr
}

func testSummaryLocalAddress(t *testing.T, path constraint.Path) flow.LocalAddress {
	t.Helper()
	addr, ok := flow.LocalAddressOfPath(path)
	if !ok {
		t.Fatalf("local address for path %s", path.Key())
	}
	return addr
}

func TestProject_ReturnProjectionPrefersIdentifierValue(t *testing.T) {
	g := returnFunctionGraph(t, "return x")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	slotValue := product.FromType(typ.Number)
	idValue := product.FromType(typ.String)

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.ReturnSlotValueKey(0):           slotValue,
					flow.SymbolValueKey(info.Symbols[0]): product.FromType(typ.Boolean),
				},
				Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: info.Symbols[0], Value: idValue}}),
			},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], idValue) {
		t.Fatalf("summary return value = %v, want identifier-backed %v", ReturnValues(sum), idValue)
	}
}

func TestProject_ReturnProjectionOverlaysIdentifierStaticMembers(t *testing.T) {
	g := returnFunctionGraph(t, "return graph")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	graphSym := info.Symbols[0]
	fieldPath := constraint.NewPath(graphSym, "graph").Field("static_data_sources")
	fieldValue := product.FromType(typ.NewArray(typ.NewRecord().
		Field("routes", typ.NewFreshArray()).
		Build()))

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(graphSym): product.FromType(typ.NewRecord().Build()),
				},
				StaticMembers: flow.StaticMemberFactsDomain.Top().
					WithAddress(testSummaryStableAddress(t, fieldPath), fieldValue),
			},
		},
	}, g)

	if len(sum.Returns) != 1 {
		t.Fatalf("summary returns = %d, want 1", len(sum.Returns))
	}
	got, ok := product.MemberOf(sum.Returns[0], value.MemberField("static_data_sources"))
	if !ok || !product.Domain.Equal(got, fieldValue) {
		t.Fatalf("returned graph.static_data_sources = %v/%v, want %v", got.ProjectValue(), ok, fieldValue.ProjectValue())
	}
	if len(sum.ReturnStaticMembers) != 1 || !sum.ReturnStaticMembers[0].HasProof() {
		t.Fatalf("return static members = %#v, want slot proof", sum.ReturnStaticMembers)
	}
	slotField := constraint.NewPlaceholder(0).Field("static_data_sources")
	if got, ok := sum.ReturnStaticMembers[0].ValueAtAddress(testSummaryStableAddress(t, slotField)); !ok || !product.Domain.Equal(got, fieldValue) {
		t.Fatalf("return static member $0.static_data_sources = %v/%v, want %v", got.ProjectValue(), ok, fieldValue.ProjectValue())
	}
}

func TestProject_ReturnStaticMembersIncludesDirectProductFields(t *testing.T) {
	g := returnFunctionGraph(t, "return graph")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	graphSym := info.Symbols[0]
	edgesValue := typ.NewMap(typ.String, typ.NewRecord().
		Field("targets", typ.NewFreshArray()).
		Field("error_targets", typ.NewFreshArray()).
		Build())

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(graphSym): product.FromType(typ.NewRecord().
						Field("edges", edgesValue).
						Build()),
				},
				StaticMembers: flow.StaticMemberFactsDomain.Top(),
			},
		},
	}, g)

	slotField := constraint.NewPlaceholder(0).Field("edges")
	got, ok := sum.ReturnStaticMembers[0].ValueAtAddress(testSummaryStableAddress(t, slotField))
	if !ok || !typ.TypeEquals(got.ProjectValue(), edgesValue) {
		t.Fatalf("return static member $0.edges = %v/%v, want %v", got.ProjectValue(), ok, edgesValue)
	}
}

func TestProject_ReturnFreshArrayMemberPublishesLengthUpperBound(t *testing.T) {
	g := returnFunctionGraph(t, "return graph")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	graphSym := info.Symbols[0]

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(graphSym): product.FromType(typ.NewRecord().
						Field("xs", typ.NewFreshArray()).
						Build()),
				},
			},
		},
	}, g)

	want := flow.BoundaryLengthUpperBound{
		Target: flow.BoundaryPath{
			Kind:     flow.BoundaryPathReturn,
			Index:    0,
			Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "xs"}},
		},
		Upper: 0,
	}
	if !sum.BoundaryFacts.HasLengthUpperBound(want) {
		t.Fatalf("summary boundary facts = %#v, want returned xs length upper %#v", sum.BoundaryFacts, want)
	}
}

func TestProject_ReturnStaticMembersDirectFieldsUseRawReturnedProduct(t *testing.T) {
	g := returnFunctionGraph(t, "return graph")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	graphSym := info.Symbols[0]
	edgesValue := typ.NewMap(typ.String, typ.NewRecord().
		Field("targets", typ.NewFreshArray()).
		Field("error_targets", typ.NewFreshArray()).
		Build())
	staleEdgesPath := constraint.NewPath(graphSym, "graph").Field("edges")

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(graphSym): product.FromType(typ.NewRecord().
						Field("edges", edgesValue).
						Build()),
				},
				StaticMembers: flow.StaticMemberFactsDomain.Top().
					WithAddress(testSummaryStableAddress(t, staleEdgesPath), product.FromType(typ.NewRecord().Build())),
			},
		},
	}, g)

	slotField := constraint.NewPlaceholder(0).Field("edges")
	got, ok := sum.ReturnStaticMembers[0].ValueAtAddress(testSummaryStableAddress(t, slotField))
	if !ok || !typ.TypeEquals(got.ProjectValue(), edgesValue) {
		t.Fatalf("return static member $0.edges = %v/%v, want raw product field %v", got.ProjectValue(), ok, edgesValue)
	}
}

func TestProject_ReturnStaticMembersRequireEveryReturnPath(t *testing.T) {
	g := returnFunctionGraphWithParams(t, []string{"flag"}, "local rich = {}\nlocal plain = {}\nif flag then return rich end\nreturn plain")

	var richRet cfg.Point
	var plainRet cfg.Point
	var richSym cfg.SymbolID
	var plainSym cfg.SymbolID
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Symbols) != 1 {
			return
		}
		ident, _ := info.Exprs[0].(*ast.IdentExpr)
		if ident == nil {
			return
		}
		switch ident.Value {
		case "rich":
			richRet = p
			richSym = info.Symbols[0]
		case "plain":
			plainRet = p
			plainSym = info.Symbols[0]
		}
	})
	if richRet == 0 || plainRet == 0 || richSym == 0 || plainSym == 0 {
		t.Fatalf("expected rich/plain returns, got rich=%v/%v plain=%v/%v", richRet, richSym, plainRet, plainSym)
	}
	richField := constraint.NewPath(richSym, "rich").Field("id")

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			richRet: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(richSym): product.FromType(typ.NewRecord().Build()),
				},
				StaticMembers: flow.StaticMemberFactsDomain.Top().
					WithAddress(testSummaryStableAddress(t, richField), product.FromType(typ.String)),
			},
			plainRet: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(plainSym): product.FromType(typ.NewRecord().Build()),
				},
				StaticMembers: flow.StaticMemberFactsDomain.Top(),
			},
		},
	}, g)

	if len(sum.ReturnStaticMembers) != 1 || sum.ReturnStaticMembers[0].HasProof() {
		t.Fatalf("one-branch return static members = %#v, want no definite proof", sum.ReturnStaticMembers)
	}
}

func TestProject_ReturnProjectionFallsBackToReturnSlot(t *testing.T) {
	g := returnFunctionGraph(t, "return x")
	ret, info := returnPointAndInfo(t, g)
	slotValue := product.FromType(typ.Number)

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.ReturnSlotValueKey(0): slotValue,
				},
			},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], slotValue) {
		t.Fatalf("summary return fallback = %v, want %v", ReturnValues(sum), slotValue)
	}
	if info.Symbols[0] == 0 {
		t.Fatalf("sanity: expected identifier return slot")
	}
}

func TestProject_ReturnProjectionUsesReturnSlotForNonIdentifier(t *testing.T) {
	g := returnFunctionGraph(t, "return 123")
	ret, _ := returnPointAndInfo(t, g)
	slotValue := product.FromType(typ.Number)

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.ReturnSlotValueKey(0): slotValue,
				},
			},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], slotValue) {
		t.Fatalf("summary return value = %v, want %v", ReturnValues(sum), slotValue)
	}
}

func TestProject_ReturnProjectionFallsBackToTop(t *testing.T) {
	g := returnFunctionGraph(t, "return 123")
	ret, _ := returnPointAndInfo(t, g)

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {Env: map[flow.ValueKey]product.AbstractValue{}},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], product.Domain.Top()) {
		t.Fatalf("summary return value = %v, want Top", ReturnValues(sum))
	}
}

func TestProject_ReturnProjectionTreatsFiniteReturnedCallTargetAsPendingBottom(t *testing.T) {
	g := returnFunctionGraph(t, "if flag then return 1 end\nreturn callee()")
	numberRet, callRet, info := splitNumberAndCallReturns(t, g)
	call := info.SourceCallAt(0)
	if call == nil {
		t.Fatal("return call info not found")
	}
	refs := flow.WithFunctionRefPath(nil, call.CalleePath, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 77}))
	numberValue := product.FromType(typ.Number)

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			numberRet: {Env: map[flow.ValueKey]product.AbstractValue{flow.ReturnSlotValueKey(0): numberValue}},
			callRet:   {FunctionRefs: refs},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], numberValue) {
		t.Fatalf("finite returned call target = %v, want number", ReturnValues(sum))
	}
}

func TestProject_ReturnProjectionUsesStaticTargetClassifierForPendingCall(t *testing.T) {
	g := returnFunctionGraph(t, "if flag then return 1 end\nreturn callee()")
	numberRet, callRet, info := splitNumberAndCallReturns(t, g)
	call := info.SourceCallAt(0)
	if call == nil {
		t.Fatal("return call info not found")
	}
	numberValue := product.FromType(typ.Number)

	sum := projectWithOptions(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			numberRet: {Env: map[flow.ValueKey]product.AbstractValue{flow.ReturnSlotValueKey(0): numberValue}},
			callRet:   {},
		},
	}, g, projectOptions{
		ReturnCallHasFiniteTarget: func(got *cfg.CallInfo) bool {
			return got == call
		},
	})

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], numberValue) {
		t.Fatalf("static returned call target = %v, want number", ReturnValues(sum))
	}
}

func TestProject_ReturnProjectionDoesNotMutateInputEnv(t *testing.T) {
	g := returnFunctionGraph(t, "return 123")
	ret, _ := returnPointAndInfo(t, g)
	slotValue := product.FromType(typ.Number)
	start := map[flow.ValueKey]product.AbstractValue{
		flow.ReturnSlotValueKey(0): slotValue,
	}
	before := cloneValueEnv(start)

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {Env: start},
		},
	}, g)
	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], slotValue) {
		t.Fatalf("summary return value = %v, want %v", ReturnValues(sum), slotValue)
	}
	if !valueEnvEqual(before, start) {
		t.Fatalf("Project mutated return-point Env: before=%v after=%v", before, start)
	}
}

func TestProject_ExportsPointLengthParamRelationForReturnedTarget(t *testing.T) {
	g := returnFunctionGraph(t, "return out")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	targetPath := constraint.Path{Symbol: info.Symbols[0]}
	targetPath.Version = g.VisibleVersion(ret, info.Symbols[0]).ID
	target := testSummaryLocalAddress(t, targetPath)
	paramIndex := 1

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Rel: flow.PointRelations{}.WithTargetLengthParamLocal(target, paramIndex),
			},
		},
	}, g)

	lengthFacts := sum.BoundaryFacts.LengthRelations()
	want := flow.BoundaryLengthRelationFact{
		Target: flow.BoundaryPath{Kind: flow.BoundaryPathReturn, Index: 0},
		Source: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: paramIndex},
	}
	if len(lengthFacts) != 1 || !summaryBoundaryLengthRelationEqual(lengthFacts[0], want) {
		t.Fatalf("summary boundary length facts = %#v, want %#v", lengthFacts, want)
	}
}

func TestProject_ExportsReturnKeyBoundaryFactForReturnedKey(t *testing.T) {
	g := returnFunctionGraphWithParams(t, []string{"self"}, "local id = \"node\"\nreturn id")
	ret, info := returnPointAndInfo(t, g)
	params := g.ParamSymbols()
	if len(params) != 1 || params[0] == 0 {
		t.Fatalf("parameter symbol not found: %#v", params)
	}
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	selfPath := constraint.NewPath(params[0], "self")
	nodesPath := selfPath.Field("nodes")
	idPath := constraint.NewPath(info.Symbols[0], "id")

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				KeyPresence: flow.KeyPresenceFacts{}.WithAddresses(
					testSummaryStableAddress(t, nodesPath),
					testSummaryStableAddress(t, idPath),
				),
			},
		},
	}, g)

	facts := sum.BoundaryFacts.KeyPresence()
	wantTable := flow.BoundaryPath{
		Kind:     flow.BoundaryPathParam,
		Index:    0,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}},
	}
	wantKey := flow.BoundaryPath{Kind: flow.BoundaryPathReturn, Index: 0}
	if len(facts) != 1 ||
		!summaryBoundaryPathEqual(facts[0].Table, wantTable) ||
		!summaryBoundaryPathEqual(facts[0].Key, wantKey) {
		t.Fatalf("summary boundary facts = %#v, want table %#v key %#v", facts, wantTable, wantKey)
	}
}

func summaryBoundaryPathEqual(a, b flow.BoundaryPath) bool {
	if a.Kind != b.Kind || a.Index != b.Index || len(a.Segments) != len(b.Segments) {
		return false
	}
	for i := range a.Segments {
		if a.Segments[i] != b.Segments[i] {
			return false
		}
	}
	return true
}

func summaryBoundaryLengthRelationEqual(a, b flow.BoundaryLengthRelationFact) bool {
	return summaryBoundaryPathEqual(a.Target, b.Target) && summaryBoundaryPathEqual(a.Source, b.Source)
}

func TestProject_RejectsStalePointLengthParamRelationKey(t *testing.T) {
	g := returnFunctionGraph(t, "return out")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	stale := testSummaryLocalAddress(t, constraint.Path{Symbol: info.Symbols[0], Version: 999})

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Rel: flow.PointRelations{}.WithTargetLengthParamLocal(stale, 1),
			},
		},
	}, g)

	if got := sum.BoundaryFacts.LengthRelations(); len(got) != 0 {
		t.Fatalf("summary boundary facts exported stale target key: %#v", got)
	}
}

func TestProject_ReturnBoundaryFactsIgnoreNilErrorReturns(t *testing.T) {
	g := returnFunctionGraphWithParams(t, []string{"err"}, "local graph = {}\nif err then return nil, err end\nreturn graph, nil")

	var failureRet cfg.Point
	var successRet cfg.Point
	var graphSym cfg.SymbolID
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Symbols) == 0 {
			return
		}
		if info.Symbols[0] != 0 {
			successRet = p
			if ident, ok := info.Exprs[0].(*ast.IdentExpr); ok {
				graphSym, _ = g.Bindings().SymbolOf(ident)
			}
		} else {
			failureRet = p
		}
	})
	if failureRet == 0 || successRet == 0 || graphSym == 0 {
		t.Fatalf("expected nil and graph return points, got failure=%v success=%v graph=%v", failureRet, successRet, graphSym)
	}
	graphPath := constraint.NewPath(graphSym, "graph")
	nodeOrderPath := graphPath.Field("node_order")
	nodesPath := graphPath.Field("nodes")

	sum := Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			failureRet: {},
			successRet: {
				KeyPresence: flow.KeyPresenceFacts{}.
					WithKeyArrayValueAddresses(
						testSummaryStableAddress(t, nodeOrderPath),
						testSummaryStableAddress(t, nodesPath),
						product.FromType(typ.String),
					),
			},
		},
	}, g)

	facts := sum.BoundaryFacts.KeyArrayValues()
	if len(facts) != 1 {
		t.Fatalf("boundary key-array values = %#v, want one return-relative fact", facts)
	}
	if facts[0].Array.Kind != flow.BoundaryPathReturn ||
		facts[0].Array.Index != 0 ||
		len(facts[0].Array.Segments) != 1 ||
		facts[0].Table.Kind != flow.BoundaryPathReturn ||
		facts[0].Table.Index != 0 ||
		len(facts[0].Table.Segments) != 1 {
		t.Fatalf("boundary fact = %#v, want return[0].node_order -> return[0].nodes", facts[0])
	}
}

func returnFunctionGraph(t *testing.T, code string) *cfg.Graph {
	return returnFunctionGraphWithParams(t, nil, code)
}

func returnFunctionGraphWithParams(t *testing.T, params []string, code string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "return_slot.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	if len(params) > 0 {
		fn.ParList = &ast.ParList{Names: params}
	}
	return cfg.Build(fn)
}

func returnPointAndInfo(t *testing.T, g *cfg.Graph) (cfg.Point, *cfg.ReturnInfo) {
	t.Helper()
	var ret cfg.Point
	var info *cfg.ReturnInfo
	g.EachReturn(func(p cfg.Point, r *cfg.ReturnInfo) {
		ret = p
		info = r
	})
	if ret == 0 || info == nil {
		t.Fatal("return point not found")
	}
	return ret, info
}

func splitNumberAndCallReturns(t *testing.T, g *cfg.Graph) (cfg.Point, cfg.Point, *cfg.ReturnInfo) {
	t.Helper()
	var numberRet cfg.Point
	var callRet cfg.Point
	var callInfo *cfg.ReturnInfo
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		if info.SourceCallAt(0) != nil {
			callRet = p
			callInfo = info
			return
		}
		numberRet = p
	})
	if numberRet == 0 || callRet == 0 || callInfo == nil {
		t.Fatalf("expected number and call return points, got number=%v call=%v info=%#v", numberRet, callRet, callInfo)
	}
	return numberRet, callRet, callInfo
}

func cloneValueEnv(in map[flow.ValueKey]product.AbstractValue) map[flow.ValueKey]product.AbstractValue {
	out := make(map[flow.ValueKey]product.AbstractValue, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func valueEnvEqual(a, b map[flow.ValueKey]product.AbstractValue) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !product.Domain.Equal(av, bv) {
			return false
		}
	}
	for k, bv := range b {
		av, ok := a[k]
		if !ok || !product.Domain.Equal(av, bv) {
			return false
		}
	}
	return true
}
