package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

func (l *lowerer) addProtectedCallBranchRefinements(input *factflow.FactsInput, graph cfg.Graph, result *semantics.Result) {
	if input == nil || graph == nil || result == nil {
		return
	}
	for _, callPoint := range graph.RPO() {
		view, ok := result.CallView(callPoint)
		if !ok {
			continue
		}
		fact, _ := view.Borrowed()
		payloadType, ok := l.protectedCallPayloadType(fact)
		if !ok {
			continue
		}
		okPath, ok := callResultTargetPath(fact, 0)
		if !ok {
			continue
		}
		payloadPath, ok := callResultTargetPath(fact, 1)
		if !ok {
			continue
		}
		targets := resultCorrelationTargets{
			triggerPath:        okPath,
			triggerResultIndex: 0,
			targetPath:         payloadPath,
			targetResultIndex:  1,
			hasTargetPath:      true,
		}
		establish, ok := resultCorrelationEstablishPoint(input, graph, callPoint, targets)
		if !ok {
			continue
		}
		activeIn := resultCorrelationActiveIn(input, graph, establish, targets)
		payloadValue := l.typeWitnessValue(payloadType)
		for _, branch := range graph.RPO() {
			if !activeIn[branch] || !graph.IsBranch(branch) {
				continue
			}
			for _, cond := range l.protectedCallSuccessEdges(result, branch, okPath) {
				appendBranchRefinement(input.BranchRefinements, branch,
					branchRefinementOnEdge(payloadPath, factflow.NewValueConstraint(payloadValue), cond),
				)
			}
		}
	}
}

func (l *lowerer) protectedCallPayloadType(fact semantics.CallFact) (typ.Type, bool) {
	if !fact.IsProtectedCall(l.bindings) || len(fact.Args) == 0 {
		return nil, false
	}
	callbackType, ok := l.expressionOperandType(fact.Args[0])
	if !ok {
		if value, vok := l.expressionValue(fact.Args[0]); vok {
			callbackType, ok = typevalue.TypeOf(l.registry, value)
		}
	}
	if !ok {
		return nil, false
	}
	return typecall.CallableReturn(callbackType)
}

func (l *lowerer) protectedCallSuccessEdges(result *semantics.Result, branch cfg.Point, okPath path.Path) []bool {
	check, ok := l.directBranchCheckAt(branch, result)
	if !ok || !check.Path.Equal(okPath) {
		return nil
	}
	switch check.Kind {
	case branchcond.CheckTruthy:
		return []bool{true}
	case branchcond.CheckFalsy:
		return []bool{false}
	default:
		return nil
	}
}
