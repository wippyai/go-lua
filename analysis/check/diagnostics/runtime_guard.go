package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func runtimeTypeName(name string) (typ.Type, bool) {
	switch name {
	case "nil":
		return typ.Nil, true
	case "boolean":
		return typ.Boolean, true
	case "number":
		return typ.Number, true
	case "string":
		return typ.String, true
	default:
		return nil, false
	}
}

func dominantRuntimeTypeGuard(result *body.Result, point cfg.Point, p path.Path, want typ.Type) bool {
	graph := result.Graph()
	if graph == nil || point == 0 || p.IsEmpty() {
		return false
	}
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	for _, branch := range graph.RPO() {
		if !dom.StrictlyDominates(branch, point) {
			continue
		}
		fact, ok := result.BranchCondition(branch)
		if !ok || !fact.Check.Path.Equal(p) {
			continue
		}
		rejectCond, ok := runtimeTypeGuardRejectCond(fact.Check.Kind)
		if !ok {
			continue
		}
		t, ok := runtimeTypeName(fact.Check.TypeName)
		if !ok || !subtype.IsSubtype(t, want) {
			continue
		}
		for _, succ := range graph.Successors(branch) {
			cond, ok := graph.EdgeCond(branch, succ)
			if !ok || cond != rejectCond {
				continue
			}
			if !cfg.PointCanReach(graph, succ, point) {
				return true
			}
		}
	}
	return false
}

func runtimeTypeGuardRejectCond(kind branchcond.CheckKind) (bool, bool) {
	switch kind {
	case branchcond.CheckTypeEqual:
		return false, true
	case branchcond.CheckTypeNot:
		return true, true
	default:
		return false, false
	}
}
