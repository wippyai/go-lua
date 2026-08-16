package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// AlwaysTrueGuardOccurrence is a reachable branch condition whose solved
// expression type is exactly the singleton true or singleton false type.
type AlwaysTrueGuardOccurrence struct {
	Point          cfg.Point
	Always         bool
	ConditionLabel string
	ConditionType  typ.Type
	ConditionSpan  SourceSpan
}

// ForEachAlwaysTrueGuardOccurrence visits branch conditions whose solved
// expression type is exactly the singleton true or singleton false type.
func (r *Result) ForEachAlwaysTrueGuardOccurrence(visit func(AlwaysTrueGuardOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	r.ForEachUserVisibleBranchConditionOccurrence(func(occ BranchConditionOccurrence) bool {
		t, ok := r.ExpressionTypeBeforeBoundary(occ.Point, occ.Fact.Condition)
		if !ok || t == nil {
			return true
		}
		always, singleton := r.singletonBoolean(t)
		if !singleton {
			return true
		}
		label := ExpressionLabel(occ.Fact.Condition)
		if label == "" {
			label = "condition"
		}
		visited = true
		return visit(AlwaysTrueGuardOccurrence{
			Point:          occ.Point,
			Always:         always,
			ConditionLabel: label,
			ConditionType:  t,
			ConditionSpan:  occ.ConditionSpan,
		})
	})
	return visited
}

func (r *Result) singletonBoolean(t typ.Type) (bool, bool) {
	if t == nil || r == nil {
		return false, false
	}
	if r.IsSubtype(t, typ.True) && r.IsSubtype(typ.True, t) {
		return true, true
	}
	if r.IsSubtype(t, typ.False) && r.IsSubtype(typ.False, t) {
		return false, true
	}
	return false, false
}
