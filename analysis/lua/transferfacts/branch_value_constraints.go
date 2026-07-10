package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/typenarrow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func (l *lowerer) typeBranchRefinement(target path.Path, kind branchcond.CheckKind, typeName string) (factflow.BranchRefinement, bool) {
	tag, ok := runtimekind.ParseTag(typeName)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	matched := typenarrow.MatchRefinement(l.registry, tag)
	unmatched := typenarrow.UnmatchRefinement(l.registry, tag)
	runtimeProof := l.runtimeAssertionValue(product.Top())
	matched = matched.WithConstraint(l.registry, runtimeProof)
	unmatched = unmatched.WithConstraint(l.registry, runtimeProof)
	if kind == branchcond.CheckTypeNot {
		return factflow.NewBranchRefinement(target, unmatched, true, matched, true), true
	}
	return factflow.NewBranchRefinement(target, matched, true, unmatched, true), true
}

func (l *lowerer) presenceRefinement(value presence.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(l.presenceConstraint(value))
}

func (l *lowerer) falsyAbsentRefinement() factflow.ValueRefinement {
	return factflow.NewFalsyAbsentConstraint(l.presenceConstraint(presence.Absent()))
}

func (l *lowerer) presenceConstraint(value presence.Value) product.Value {
	return product.NewWithPresence(l.registry, product.ShapeTop, value)
}
