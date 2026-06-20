package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// WriteDiffConstraint records value(hi) - value(lo) <= c as a must-fact proven
// on this path. A length operand is spelled with LengthRelKey(arrayKey). It is a
// thin wrapper over the generalized relation lane with an empty B operand.
func (s State) WriteDiffConstraint(hi, lo pathdom.PathKey, c int64) State {
	return s.writeRelConstraint(RelConstraint{CoA: 1, A: hi, C: lo, K: c})
}

// WriteSumConstraint records value(a) + value(b) - value(c) <= k as a must-fact
// proven on this path. A length operand is spelled with LengthRelKey(arrayKey).
func (s State) WriteSumConstraint(a, b, c pathdom.PathKey, k int64) State {
	return s.writeRelConstraint(RelConstraint{CoA: 1, A: a, CoB: 1, B: b, C: c, K: k})
}

// WriteScaledConstraint records coA*value(a) + coB*value(b) - value(c) <= k as a
// must-fact proven on this path, the scaled form of WriteSumConstraint. An empty
// b drops the second positive term, giving coA*value(a) - value(c) <= k. A length
// operand is spelled with LengthRelKey(arrayKey).
func (s State) WriteScaledConstraint(coA int64, a pathdom.PathKey, coB int64, b pathdom.PathKey, c pathdom.PathKey, k int64) State {
	return s.writeRelConstraint(RelConstraint{CoA: coA, A: a, CoB: coB, B: b, C: c, K: k})
}

func (s State) writeRelConstraint(c RelConstraint) State {
	lane, changed := s.diffRelations.add(c)
	if !changed {
		return s
	}
	out := s.reachable()
	out.diffRelations = lane
	return out
}

// RelConstraintsSnapshot is a stable snapshot of the relational-constraint lane.
type RelConstraintsSnapshot struct {
	Bottom      bool
	Top         bool
	Constraints []RelConstraint
}

// RelConstraints returns the relational constraints proven at this state.
func (s State) RelConstraints() RelConstraintsSnapshot {
	bottom, top, items := s.diffRelations.snapshot(relConstraintLess)
	return RelConstraintsSnapshot{Bottom: bottom, Top: top, Constraints: items}
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

func relConstraintLess(a, b RelConstraint) bool {
	if a.A != b.A {
		return a.A < b.A
	}
	if a.B != b.B {
		return a.B < b.B
	}
	if a.C != b.C {
		return a.C < b.C
	}
	if a.CoA != b.CoA {
		return a.CoA < b.CoA
	}
	if a.CoB != b.CoB {
		return a.CoB < b.CoB
	}
	return a.K < b.K
}
