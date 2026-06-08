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
		return constraint.ConstraintAffectedByWrite(c, path)
	})
	if constraint.Domain.Equal(out.Cond, next) {
		return false
	}
	out.Cond = next
	return true
}
