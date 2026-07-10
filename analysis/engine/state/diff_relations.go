package state

import (
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
)

// WriteDiffConstraint records value(hi) - value(lo) <= c as a must-fact proven
// on this path. It is a thin wrapper over the generalized relation lane with an
// empty B operand.
func (s State) WriteDiffConstraint(hi, lo RelOperand, c int64) State {
	return s.writeRelConstraint(RelConstraint{CoA: 1, A: hi, C: lo, K: c})
}

// WriteSumConstraint records value(a) + value(b) - value(c) <= k as a must-fact
// proven on this path.
func (s State) WriteSumConstraint(a, b, c RelOperand, k int64) State {
	return s.writeRelConstraint(RelConstraint{CoA: 1, A: a, CoB: 1, B: b, C: c, K: k})
}

// WriteScaledConstraint records coA*value(a) + coB*value(b) - value(c) <= k as a
// must-fact proven on this path, the scaled form of WriteSumConstraint. An empty
// b drops the second positive term, giving coA*A - C <= k.
func (s State) WriteScaledConstraint(coA int64, a RelOperand, coB int64, b RelOperand, c RelOperand, k int64) State {
	return s.writeRelConstraint(RelConstraint{CoA: coA, A: a, CoB: coB, B: b, C: c, K: k})
}

func (s State) writeRelConstraint(c RelConstraint) State {
	if !s.laneEnabled(laneDiffRelationsBit) {
		return s
	}
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
	if !s.laneEnabled(laneDiffRelationsBit) {
		return RelConstraintsSnapshot{Bottom: true}
	}
	bottom, top, items := s.diffRelations.snapshot(relConstraintLess)
	return RelConstraintsSnapshot{Bottom: bottom, Top: top, Constraints: items}
}

// ClearDiffConstraintsFor drops every constraint mentioning key, whether as a
// value operand or as the array behind a length operand. It is used when a root
// symbol is reassigned, since all prior relations over its old value are stale.
func (s State) ClearDiffConstraintsFor(key pathaddr.StateKey) State {
	if key == "" || !s.laneEnabled(laneDiffRelationsBit) {
		return s
	}
	lane, changed := s.diffRelations.clearMatching(func(k pathaddr.StateKey) bool {
		return k == key
	})
	if !changed {
		return s
	}
	out := s.reachable()
	out.diffRelations = lane
	return out
}

// NumericConstraint returns the flattened numeric-solver view of this relation.
// The solver sees opaque variables; typed length operands are encoded only at
// this adapter boundary, never in the state lane itself.
func (c RelConstraint) NumericConstraint() numeric.SumLe {
	return numeric.NewScaledLe(
		c.CoA, relOperandNumericKey(c.A),
		c.CoB, relOperandNumericKey(c.B),
		relOperandNumericKey(c.C),
		c.K,
	)
}

// AppendValueStateKeys appends the value(path) operands mentioned by this
// constraint to out and returns the extended slice.
// Length operands are intentionally excluded because numeric floors constrain
// values, not lengths.
func (c RelConstraint) AppendValueStateKeys(out []pathaddr.StateKey) []pathaddr.StateKey {
	if c.A.Kind == RelOperandValue && c.A.Key != "" {
		out = append(out, c.A.Key)
	}
	if c.B.Kind == RelOperandValue && c.B.Key != "" {
		out = append(out, c.B.Key)
	}
	if c.C.Kind == RelOperandValue && c.C.Key != "" {
		out = append(out, c.C.Key)
	}
	return out
}

// NumericKey returns the opaque solver variable for this operand.
func (o RelOperand) NumericKey() pathdom.PathKey {
	return relOperandNumericKey(o)
}

const relLengthSolverPrefix = "\x00len:"

func relOperandNumericKey(o RelOperand) pathdom.PathKey {
	if !o.valid() {
		return ""
	}
	if o.Kind == RelOperandLength {
		return pathdom.PathKey(relLengthSolverPrefix + string(o.Key.PathKey()))
	}
	return o.Key.PathKey()
}

func relConstraintLess(a, b RelConstraint) bool {
	if a.A != b.A {
		return relOperandLess(a.A, b.A)
	}
	if a.B != b.B {
		return relOperandLess(a.B, b.B)
	}
	if a.C != b.C {
		return relOperandLess(a.C, b.C)
	}
	if a.CoA != b.CoA {
		return a.CoA < b.CoA
	}
	if a.CoB != b.CoB {
		return a.CoB < b.CoB
	}
	return a.K < b.K
}
