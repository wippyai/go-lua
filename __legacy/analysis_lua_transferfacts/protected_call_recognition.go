package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typecall"
)

func (l *lowerer) addProtectedCallBranchRefinements(input *factflow.FactsInput, graph cfg.Graph) {
	if input == nil || graph == nil {
		return
	}
	for _, callPoint := range graph.RPO() {
		site, ok := input.CallSites[callPoint]
		if !ok {
			continue
		}
		siteView := site.View()
		payloadType, ok := l.protectedCallPayloadTypeFromWIRCallSite(callPoint, siteView)
		if !ok {
			continue
		}
		okPath, ok := callSiteResultTargetPath(siteView, 0)
		if !ok {
			continue
		}
		payloadPath, ok := callSiteResultTargetPath(siteView, 1)
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

func (l *lowerer) protectedCallPayloadTypeFromWIRCallSite(point cfg.Point, site factflow.CallSiteView) (typ.Type, bool) {
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

func (l *lowerer) isProtectedCallSite(site factflow.CallSiteView) bool {
	return l.isNamedGlobalCallSite(site, "pcall") || l.isNamedGlobalCallSite(site, "xpcall")
}

func (l *lowerer) isNamedGlobalCallSite(site factflow.CallSiteView, name string) bool {
	if l == nil || l.wir == nil || name == "" {
		return false
	}
	global, ok := l.wir.GlobalSymbol(name)
	return ok && site.CalleeSymbol() == global
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
