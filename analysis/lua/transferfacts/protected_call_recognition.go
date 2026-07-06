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
	if input == nil || graph == nil {
		return
	}
	if l != nil && l.wir != nil {
		l.addProtectedCallBranchRefinementsFromWIR(input, graph)
		return
	}
	if result == nil {
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
		okPath, ok := l.callResultTargetPath(callPoint, fact, 0)
		if !ok {
			continue
		}
		payloadPath, ok := l.callResultTargetPath(callPoint, fact, 1)
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
			for _, cond := range l.semanticProtectedCallSuccessEdges(result, branch, okPath) {
				appendBranchRefinement(input.BranchRefinements, branch,
					branchRefinementOnEdge(payloadPath, factflow.NewValueConstraint(payloadValue), cond),
				)
			}
		}
	}
}

func (l *lowerer) addProtectedCallBranchRefinementsFromWIR(input *factflow.FactsInput, graph cfg.Graph) {
	if l == nil || l.wir == nil || input == nil || graph == nil {
		return
	}
	for _, callPoint := range graph.RPO() {
		site, ok := input.CallSites[callPoint]
		if !ok {
			continue
		}
		payloadType, ok := l.protectedCallPayloadTypeFromWIRCallSite(callPoint, site)
		if !ok {
			continue
		}
		okPath, ok := callSiteResultTargetPath(site, 0)
		if !ok {
			continue
		}
		payloadPath, ok := callSiteResultTargetPath(site, 1)
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
			for _, cond := range l.wirProtectedCallSuccessEdges(branch, okPath) {
				appendBranchRefinement(input.BranchRefinements, branch,
					branchRefinementOnEdge(payloadPath, factflow.NewValueConstraint(payloadValue), cond),
				)
			}
		}
	}
}

func (l *lowerer) protectedCallPayloadTypeFromWIRCallSite(point cfg.Point, site factflow.CallSite) (typ.Type, bool) {
	if l == nil || l.wir == nil || !l.isProtectedCallSite(site) {
		return nil, false
	}
	callbackPath, ok := l.callArgumentPathFromWIR(point, 0)
	if !ok {
		return nil, false
	}
	callbackType, ok := l.aliasPathType(callbackPath)
	if !ok {
		return nil, false
	}
	return typecall.CallableReturn(callbackType)
}

func (l *lowerer) isProtectedCallSite(site factflow.CallSite) bool {
	return l.isNamedGlobalCallSite(site, "pcall") || l.isNamedGlobalCallSite(site, "xpcall")
}

func (l *lowerer) isNamedGlobalCallSite(site factflow.CallSite, name string) bool {
	if l == nil || l.bindings == nil || name == "" {
		return false
	}
	global, ok := l.bindings.GlobalSymbol(name)
	return ok && site.CalleeSymbol() == global
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

func (l *lowerer) semanticProtectedCallSuccessEdges(result *semantics.Result, branch cfg.Point, okPath path.Path) []bool {
	check, ok := l.semanticDirectBranchCheckAt(branch, result)
	return protectedCallSuccessEdgesForCheck(check, ok, okPath)
}

func (l *lowerer) wirProtectedCallSuccessEdges(branch cfg.Point, okPath path.Path) []bool {
	check, ok := l.directBranchCheckFromWIR(branch)
	return protectedCallSuccessEdgesForCheck(check, ok, okPath)
}

func protectedCallSuccessEdgesForCheck(check branchcond.Check, ok bool, okPath path.Path) []bool {
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
