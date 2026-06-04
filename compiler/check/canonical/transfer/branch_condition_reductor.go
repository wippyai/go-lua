package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractcond "github.com/wippyai/go-lua/compiler/check/domain/cond"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// branchConditionReductor owns the condition-axis reduction for a branch edge.
// It is transfer-owned because it needs transfer's symbol resolver and const
// resolver, but it memoizes only point/expression path lowering, which is
// independent of the changing abstract value state.
type branchConditionReductor struct {
	transfer  *Transfer
	pathCache map[abstractcond.PathCacheKey]constraint.Path
}

func (r *branchConditionReductor) effect(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken bool) (ConditionEffect, bool) {
	if r == nil || r.transfer == nil || info == nil || info.Condition == nil || r.transfer.in.Graph == nil {
		return ConditionEffect{}, false
	}
	t := r.transfer
	extractor := abstractcond.ConditionExtractor{
		P:             point,
		Inputs:        t.conditionExtractorInputs(),
		SymResolver:   t.conditionSymbolResolver(&out),
		ConstResolver: t.constResolverAt(point),
		PathCache:     r.pathCache,
	}
	branches := extractor.ConstraintsFromConditionExpr(info.Condition)
	cond := branches.OnFalse
	if taken {
		cond = branches.OnTrue
	}
	if !cond.HasConstraints() {
		return ConditionEffect{}, false
	}
	return ConditionEffect{Fact: cond}, true
}
