package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	checkstore "github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestStoreFactsFromResult_NilStore(t *testing.T) {
	StoreFactsFromResult(nil, nil, nil, nil, typ.NewRecursiveFamilyInterner())
}

func TestStoreFactsFromResult_NilResult(t *testing.T) {
	StoreFactsFromResult(nil, nil, nil, nil, typ.NewRecursiveFamilyInterner())
}

func TestStoreFactsFromResult_NilGraph(t *testing.T) {
	result := &api.FuncResult{}
	StoreFactsFromResult(nil, nil, result, nil, typ.NewRecursiveFamilyInterner())
}

func TestCollectParameterEvidenceFromResult_UsesSolvedObservationWithoutNarrowSynth(t *testing.T) {
	stmts, err := parse.ParseString(`target("value")`, "postflow_observation.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	caller := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	bindings := bind.Bind(caller, []string{"target"})
	graph := cfg.BuildWithBindings(caller, bindings)
	if graph == nil {
		t.Fatal("expected caller graph")
	}

	var callPoint cfg.Point
	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if callInfo == nil {
			callPoint = p
			callInfo = info
		}
	})
	if callInfo == nil || callInfo.CalleeSymbol == 0 {
		t.Fatalf("expected target callsite with callee symbol, got %+v", callInfo)
	}

	callee := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"arg"}}}
	calleeGraph := cfg.Build(callee)
	parent := scopeForPostflowTest()
	st := checkstore.NewSessionStore()
	st.RegisterGraph(graph, caller)
	st.RegisterGraph(calleeGraph, callee)
	st.SetParentScope(parent.Hash(), parent)
	st.SetGraphParentHash(calleeGraph.ID(), parent.Hash())
	st.RegisterFunctionRef(callInfo.CalleeSymbol, callee, calleeGraph, graph.ID(), callPoint)

	result := &api.FuncResult{
		Graph: graph,
		Evidence: api.FlowEvidence{
			Calls: []api.CallEvidence{{Point: callPoint, Info: callInfo}},
		},
	}
	CollectParameterEvidenceFromResult(st, result, parent, 0)

	key := api.KeyForGraph(graph, parent.Hash())
	facts := st.InterprocNext.Facts[key].FunctionFacts
	got := facts[callInfo.CalleeSymbol].EntryParams
	if len(got) != 1 || got[0].IsZero() {
		t.Fatalf("expected call-entry parameter evidence without NarrowSynth, got %#v", got)
	}
	if !typ.TypeEquals(got[0].ProjectValue(), typ.String) {
		t.Fatalf("entry param = %v, want string", got[0])
	}
}

func TestCollectParameterEvidenceFromResult_CanonicalProjectionSkipsLegacyBodyPreconditions(t *testing.T) {
	stmts, err := parse.ParseString(`target(page.data_func)`, "postflow_canonical.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	caller := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"page"}}, Stmts: stmts}
	graph := cfg.Build(caller, "target")
	if graph == nil {
		t.Fatal("expected caller graph")
	}

	var callPoint cfg.Point
	var callInfo *cfg.CallInfo
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if callInfo == nil {
			callPoint = p
			callInfo = info
		}
	})
	if callInfo == nil {
		t.Fatal("expected target callsite")
	}

	parent := scopeForPostflowTest()
	st := checkstore.NewSessionStore()
	const currentSym = cfg.SymbolID(900)
	st.RegisterGraph(graph, caller)
	st.SetParentScope(parent.Hash(), parent)
	st.SetGraphParentHash(graph.ID(), parent.Hash())
	st.RegisterFunctionRef(currentSym, caller, graph, 0, 0)

	result := &api.FuncResult{
		Graph:          graph,
		FlowProjection: noopFlowOps{},
		Evidence: api.FlowEvidence{
			Calls: []api.CallEvidence{{
				Point:        callPoint,
				Info:         callInfo,
				ExpectedArgs: []typ.Type{typ.String},
			}},
		},
	}
	CollectParameterEvidenceFromResult(st, result, parent, currentSym)

	key, ok := st.ParentGraphKeyForSymbol(currentSym)
	if !ok {
		t.Fatal("expected current function graph key")
	}
	ff := st.InterprocNext.Facts[key].FunctionFacts[currentSym]
	if len(ff.Params) != 0 || len(ff.BodyParams) != 0 {
		t.Fatalf("flow result must not write retired body/public params, got public=%v body=%v", ff.Params, ff.BodyParams)
	}
}

func scopeForPostflowTest() *scope.State {
	return scope.New()
}

type noopFlowOps struct{}

func (noopFlowOps) NarrowedTypeAt(cfg.Point, constraint.Path) typ.Type {
	return nil
}

func (noopFlowOps) NarrowedTypeAtWithCondition(cfg.Point, constraint.Path, constraint.Condition) typ.Type {
	return nil
}

func (noopFlowOps) PreStateTypeAt(cfg.Point, constraint.Path) typ.Type {
	return nil
}

func (noopFlowOps) ExcludesTypeAt(cfg.Point, constraint.Path, typ.Type) bool {
	return false
}

func (noopFlowOps) NumericBoundsAt(cfg.Point, cfg.SymbolID) (int64, int64, bool) {
	return 0, 0, false
}

func (noopFlowOps) ArrayLenRefPathAt(cfg.Point, cfg.SymbolID) (constraint.Path, int64, bool) {
	return constraint.Path{}, 0, false
}

func (noopFlowOps) LengthBoundsAt(cfg.Point, constraint.Path) (int64, int64, bool) {
	return 0, 0, false
}

func (noopFlowOps) IsPointDead(cfg.Point) bool {
	return false
}

func (noopFlowOps) HasKeyOf(cfg.Point, constraint.Path, constraint.Path) bool {
	return false
}

func (noopFlowOps) IndexReadback(flow.IndexWriteReadQuery) (typ.Type, bool) {
	return nil, false
}
