package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProject_ReturnProjectionPrefersIdentifierValue(t *testing.T) {
	g := returnFunctionGraph(t, "return x")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	slotValue := product.FromType(typ.Number)
	idValue := product.FromType(typ.String)

	sum := summary.Project(state.FunctionState{
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
		t.Fatalf("summary return value = %v, want identifier-backed %v", summary.ReturnValues(sum), idValue)
	}
}

func TestProject_ReturnProjectionFallsBackToReturnSlot(t *testing.T) {
	g := returnFunctionGraph(t, "return x")
	ret, info := returnPointAndInfo(t, g)
	slotValue := product.FromType(typ.Number)

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.ReturnSlotValueKey(0): slotValue,
				},
			},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], slotValue) {
		t.Fatalf("summary return fallback = %v, want %v", summary.ReturnValues(sum), slotValue)
	}
	if info.Symbols[0] == 0 {
		t.Fatalf("sanity: expected identifier return slot")
	}
}

func TestProject_ReturnProjectionUsesReturnSlotForNonIdentifier(t *testing.T) {
	g := returnFunctionGraph(t, "return 123")
	ret, _ := returnPointAndInfo(t, g)
	slotValue := product.FromType(typ.Number)

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.ReturnSlotValueKey(0): slotValue,
				},
			},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], slotValue) {
		t.Fatalf("summary return value = %v, want %v", summary.ReturnValues(sum), slotValue)
	}
}

func TestProject_ReturnProjectionFallsBackToTop(t *testing.T) {
	g := returnFunctionGraph(t, "return 123")
	ret, _ := returnPointAndInfo(t, g)

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {Env: map[flow.ValueKey]product.AbstractValue{}},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], product.Domain.Top()) {
		t.Fatalf("summary return value = %v, want Top", summary.ReturnValues(sum))
	}
}

func TestProject_ReturnProjectionTreatsFiniteReturnedCallTargetAsPendingBottom(t *testing.T) {
	g := returnFunctionGraph(t, "if flag then return 1 end\nreturn callee()")
	numberRet, callRet, info := splitNumberAndCallReturns(t, g)
	call := info.SourceCallAt(0)
	if call == nil {
		t.Fatal("return call info not found")
	}
	refs := flow.WithFunctionRef(nil, call.CalleePath.Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 77}))
	numberValue := product.FromType(typ.Number)

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			numberRet: {Env: map[flow.ValueKey]product.AbstractValue{flow.ReturnSlotValueKey(0): numberValue}},
			callRet:   {FunctionRefs: refs},
		},
	}, g)

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], numberValue) {
		t.Fatalf("finite returned call target = %v, want number", summary.ReturnValues(sum))
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

	sum := summary.ProjectWithOptions(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			numberRet: {Env: map[flow.ValueKey]product.AbstractValue{flow.ReturnSlotValueKey(0): numberValue}},
			callRet:   {},
		},
	}, g, summary.ProjectOptions{
		ReturnCallHasFiniteTarget: func(got *cfg.CallInfo) bool {
			return got == call
		},
	})

	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], numberValue) {
		t.Fatalf("static returned call target = %v, want number", summary.ReturnValues(sum))
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

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {Env: start},
		},
	}, g)
	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], slotValue) {
		t.Fatalf("summary return value = %v, want %v", summary.ReturnValues(sum), slotValue)
	}
	if !valueEnvEqual(before, start) {
		t.Fatalf("Project mutated return-point Env: before=%v after=%v", before, start)
	}
}

func TestProject_ForwardsTailCallReturnLengthRelations(t *testing.T) {
	g := returnFunctionGraph(t, "return callee()")
	ret, _ := returnPointAndInfo(t, g)
	rel := flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 1}

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				ReturnRel: flow.ReturnRelationsOfLengthParams([]flow.ReturnLengthParamRelation{rel}),
			},
		},
	}, g)

	if !sum.Relations.HasLengthParam(rel) {
		t.Fatalf("summary relations = %#v, want forwarded tail-call length relation %#v", sum.Relations, rel)
	}
}

func TestProject_DoesNotForwardStaleReturnRelationsFromNonCallReturn(t *testing.T) {
	g := returnFunctionGraph(t, "return x")
	ret, _ := returnPointAndInfo(t, g)
	rel := flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 1}

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				ReturnRel: flow.ReturnRelationsOfLengthParams([]flow.ReturnLengthParamRelation{rel}),
			},
		},
	}, g)

	if sum.Relations.HasLengthParam(rel) {
		t.Fatalf("summary relations forwarded non-call ReturnRel: %#v", sum.Relations)
	}
}

func TestProject_ExportsPointLengthParamRelationForReturnedTarget(t *testing.T) {
	g := returnFunctionGraph(t, "return out")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	targetKey := pathkey.NewResolver(g).KeyAt(ret, constraint.Path{Symbol: info.Symbols[0]})
	rel := flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 1}

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Rel: flow.PointRelations{}.WithTargetLengthParam(info.Symbols[0], targetKey, rel.ParamIndex),
			},
		},
	}, g)

	if !sum.Relations.HasLengthParam(rel) {
		t.Fatalf("summary relations = %#v, want point-local length relation %#v", sum.Relations, rel)
	}
}

func TestProject_ExportsReturnKeyParamRelationForReturnedKey(t *testing.T) {
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
	rel := flow.ReturnKeyParamRelation{
		ReturnIndex: 0,
		ParamIndex:  0,
		ParamSegments: []constraint.Segment{{
			Kind: constraint.SegmentField,
			Name: "nodes",
		}},
	}

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				KeyPresence: flow.KeyPresenceFacts{}.WithPaths(nodesPath, idPath),
			},
		},
	}, g)

	if !sum.Relations.HasKeyParam(rel) {
		t.Fatalf("summary relations = %#v, want return-key relation %#v", sum.Relations, rel)
	}
}

func TestProject_RejectsStalePointLengthParamRelationKey(t *testing.T) {
	g := returnFunctionGraph(t, "return out")
	ret, info := returnPointAndInfo(t, g)
	if len(info.Symbols) != 1 || info.Symbols[0] == 0 {
		t.Fatalf("identifier return info not found: %#v", info.Symbols)
	}
	staleKey := pathkey.NewResolver(g).KeyAtVersion(info.Symbols[0], 999, nil)
	rel := flow.ReturnLengthParamRelation{ReturnIndex: 0, ParamIndex: 1}

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Rel: flow.PointRelations{}.WithTargetLengthParam(info.Symbols[0], staleKey, rel.ParamIndex),
			},
		},
	}, g)

	if sum.Relations.HasLengthParam(rel) {
		t.Fatalf("summary relations exported stale target key: %#v", sum.Relations)
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

	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			failureRet: {},
			successRet: {
				KeyPresence: flow.KeyPresenceFacts{}.
					WithKeyArrayValuePaths(nodeOrderPath, nodesPath, product.FromType(typ.String)),
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
