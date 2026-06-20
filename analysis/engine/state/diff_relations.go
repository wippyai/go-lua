package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// WriteDiffConstraint records value(hi) - value(lo) <= c as a must-fact proven
// on this path. A length operand is spelled with LengthRelKey(arrayKey).
func (s State) WriteDiffConstraint(hi, lo pathdom.PathKey, c int64) State {
	lane, changed := s.diffRelations.add(DiffConstraint{Hi: hi, Lo: lo, C: c})
	if !changed {
		return s
	}
	out := s.reachable()
	out.diffRelations = lane
	return out
}

// DiffConstraintsSnapshot is a stable snapshot of the difference-constraint lane.
type DiffConstraintsSnapshot struct {
	Bottom      bool
	Top         bool
	Constraints []DiffConstraint
}

// DiffConstraints returns the difference constraints proven at this state.
func (s State) DiffConstraints() DiffConstraintsSnapshot {
	bottom, top, items := s.diffRelations.snapshot(diffConstraintLess)
	return DiffConstraintsSnapshot{Bottom: bottom, Top: top, Constraints: items}
}

// ClearDiffConstraintsFor drops every constraint mentioning key, whether as a
// value operand or as the array behind a length operand. It is used when a root
// symbol is reassigned, since all prior relations over its old value are stale.
func (s State) ClearDiffConstraintsFor(key pathdom.PathKey) State {
	if key == "" {
		return s
	}
	lane, changed := s.diffRelations.clearMatching(func(k pathdom.PathKey) bool {
		return k == key
	})
	if !changed {
		return s
	}
	out := s.reachable()
	out.diffRelations = lane
	return out
}

func diffConstraintLess(a, b DiffConstraint) bool {
	if a.Hi != b.Hi {
		return a.Hi < b.Hi
	}
	if a.Lo != b.Lo {
		return a.Lo < b.Lo
	}
	return a.C < b.C
}
