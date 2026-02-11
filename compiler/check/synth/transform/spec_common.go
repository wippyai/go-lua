package transform

import "github.com/wippyai/go-lua/types/constraint"

func conditionAnyDisjunctMatches(when constraint.Condition, match func(constraint.Constraint) bool) bool {
	if when.IsTrue() {
		return true
	}
	if when.IsFalse() {
		return false
	}

	for i := 0; i < when.NumDisjuncts(); i++ {
		disjunctMatches := true
		for _, c := range when.DisjunctConstraints(i) {
			if !match(c) {
				disjunctMatches = false
				break
			}
		}
		if disjunctMatches {
			return true
		}
	}
	return false
}

func placeholderArgIndex(path constraint.Path, argCount int) (int, bool) {
	if !path.IsPlaceholder() {
		return 0, false
	}
	idx := path.PlaceholderIndex()
	if idx < 0 || idx >= argCount {
		return 0, false
	}
	return idx, true
}
