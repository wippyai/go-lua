package flow

import "github.com/wippyai/go-lua/types/constraint"

func ApplyConditionFact(out *PointState, fact constraint.Condition) bool {
	if out == nil {
		return false
	}
	if fact.IsFalse() {
		*out = PointStateDomain.Bottom()
		return true
	}
	if !fact.HasConstraints() {
		return false
	}
	if out.Cond.IsFalse() || out.Cond.IsTrue() {
		out.Cond = fact
		return true
	}
	next := constraint.And(out.Cond, fact)
	if next.IsFalse() {
		*out = PointStateDomain.Bottom()
		return true
	}
	if constraint.Domain.Equal(out.Cond, next) {
		return false
	}
	out.Cond = next
	return true
}

func ForgetConditionAffectedByWrite(out *PointState, path constraint.Path) bool {
	if out == nil || out.Cond.IsFalse() || out.Cond.IsTrue() || path.Symbol == 0 {
		return false
	}
	next := out.Cond.Forget(func(c constraint.Constraint) bool {
		return conditionConstraintAffectedByWrite(c, path)
	})
	if constraint.Domain.Equal(out.Cond, next) {
		return false
	}
	out.Cond = next
	return true
}

func conditionConstraintAffectedByWrite(c constraint.Constraint, writePath constraint.Path) bool {
	for _, p := range constraint.SemanticAffectedPaths(c) {
		if conditionPathAffectedByWrite(p, writePath) {
			return true
		}
	}
	return false
}

func conditionPathAffectedByWrite(path, writePath constraint.Path) bool {
	if path.Symbol == 0 || writePath.Symbol == 0 || path.Symbol != writePath.Symbol {
		return false
	}
	if len(writePath.Segments) > len(path.Segments) {
		return false
	}
	for i := range writePath.Segments {
		if !conditionSegmentsEqual(writePath.Segments[i], path.Segments[i]) {
			return false
		}
	}
	return true
}

func conditionSegmentsEqual(a, b constraint.Segment) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.Index == b.Index
}
