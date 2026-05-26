package assign

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractCallContractAssignments lowers callee body preconditions into normal-
// return edge conditions. Consumer requirements are not producer evidence at
// the call point; they only refine caller state after the call returns normally.
func ExtractCallContractAssignments(fc *abstractcore.FlowContext, inputs *flow.Inputs) {
	if fc == nil || inputs == nil || fc.Graph == nil || len(fc.Evidence.Calls) == 0 || len(fc.FunctionFacts) == 0 {
		return
	}
	bindings := callContractBindings(fc.Graph, fc.ModuleBindings)
	if bindings == nil {
		return
	}
	hasFact := func(sym cfg.SymbolID) bool {
		_, ok := functionfact.Lookup(fc.FunctionFacts, sym)
		return ok
	}
	for _, call := range fc.Evidence.Calls {
		info := call.Info
		if info == nil {
			continue
		}
		effectPoints := normalReturnEffectPoints(fc.Graph, call.Point)
		if len(effectPoints) == 0 {
			continue
		}
		calleeSym := callsite.SelectPreferredSymbol(
			callsite.CallableCalleeSymbolCandidates(info, fc.Graph, bindings, fc.ModuleBindings),
			hasFact,
		)
		if calleeSym == 0 {
			continue
		}
		bodyParams := functionfact.BodyContractEvidence(fc.FunctionFacts, calleeSym)
		for runtimeIdx, expected := range bodyParams {
			if !paramevidence.HardPublicEvidence(expected) {
				continue
			}
			argPath := flowpath.FromExprWithBindings(callsite.RuntimeArgAt(info, runtimeIdx), nil, bindings)
			if argPath.Symbol == 0 {
				continue
			}
			condition, ok := callContractCondition(inputs, argPath, expected)
			if !ok {
				continue
			}
			for _, point := range effectPoints {
				inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{
					From:      call.Point,
					To:        point,
					Condition: condition,
				})
			}
		}
	}
}

func callContractCondition(inputs *flow.Inputs, path constraint.Path, expected typ.Type) (constraint.Condition, bool) {
	if path.Symbol == 0 || typ.IsAbsentOrUnknown(expected) || expected.Kind().IsPlaceholder() {
		return constraint.Condition{}, false
	}
	key := narrow.HashTypeKey(expected.Hash())
	if key.IsZero() {
		return constraint.Condition{}, false
	}
	if inputs.TypeKeys == nil {
		inputs.TypeKeys = make(map[uint64]typ.Type, 1)
	}
	inputs.TypeKeys[key.Hash] = expected
	return constraint.FromConstraints(constraint.HasType{Path: path, Type: key}), true
}

func normalReturnEffectPoints(graph *cfg.Graph, point cfg.Point) []cfg.Point {
	if graph == nil {
		return nil
	}
	succs := graph.Successors(point)
	if len(succs) == 0 {
		return nil
	}
	out := make([]cfg.Point, 0, len(succs))
	for _, succ := range succs {
		if succ == point {
			continue
		}
		out = append(out, succ)
	}
	return out
}

func callContractBindings(graph *cfg.Graph, moduleBindings *bind.BindingTable) *bind.BindingTable {
	if graph != nil {
		if bindings := graph.Bindings(); bindings != nil {
			return bindings
		}
	}
	return moduleBindings
}
