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

func returnFunctionGraph(t *testing.T, code string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "return_slot.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return cfg.Build(&ast.FunctionExpr{Stmts: stmts})
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
