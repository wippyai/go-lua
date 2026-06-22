package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
)

// RelConstraint records relational evidence proven on all paths reaching this
// state: CoA*value(A) + CoB*value(B) - value(C) <= K. When B is empty it is an
// ordinary difference CoA*A - C <= K; when B is set it is a bounded affine
// relation between two positive operands (with positive coefficients) and one
// negative unit-coefficient operand. Operands are typed as either value(path) or
// len(path), so guards like i < j, i + 1 <= #xs, #a == #b, i + j <= #xs, and
// 2*i <= #xs are captured without encoding length as a fake path key. The lane
// is a must-set: join is intersection, so it carries only facts proven on every
// incoming edge and converges without weight widening.
type RelConstraint struct {
	CoA int64
	A   RelOperand
	CoB int64
	B   RelOperand
	C   RelOperand
	K   int64
}

// RelOperandKind classifies the term a relational constraint operand denotes.
type RelOperandKind uint8

const (
	RelOperandInvalid RelOperandKind = iota
	RelOperandValue
	RelOperandLength
)

// RelOperand is a typed relational term over a validated state path.
type RelOperand struct {
	Key  pathaddr.StateKey
	Kind RelOperandKind
}

// RelValueOperand returns the relational operand for value(key).
func RelValueOperand(key pathaddr.StateKey) RelOperand {
	return RelOperand{Key: key, Kind: RelOperandValue}
}

// RelLengthOperand returns the relational operand for len(key).
func RelLengthOperand(key pathaddr.StateKey) RelOperand {
	return RelOperand{Key: key, Kind: RelOperandLength}
}

func (o RelOperand) valid() bool {
	return o.Key != "" && (o.Kind == RelOperandValue || o.Kind == RelOperandLength)
}

// IsValid reports whether the operand carries a value or length term.
func (o RelOperand) IsValid() bool {
	return o.valid()
}

// IsLength reports whether the operand denotes len(path) rather than value(path).
func (o RelOperand) IsLength() bool {
	return o.Kind == RelOperandLength
}

// StateKey returns the validated state path behind the relational operand.
func (o RelOperand) StateKey() pathaddr.StateKey {
	return o.Key
}

type diffRelationLane struct {
	mustSetLane[RelConstraint]
}

func diffRelationDomain() lattice.Lattice[diffRelationLane] {
	return wrapDomain(lift.MustSet[RelConstraint](), diffRelationLaneFromMustSet, diffRelationLane.asMustSet)
}

func diffRelationLaneFromMustSet(l lift.MustSetLane[RelConstraint]) diffRelationLane {
	return diffRelationLane{mustSetLaneFromLift(l)}
}

func (l diffRelationLane) add(c RelConstraint) (diffRelationLane, bool) {
	if !c.A.valid() || !c.C.valid() || c.A == c.C {
		return l, false
	}
	if c.B.valid() {
		if relOperandLess(c.B, c.A) || (c.A == c.B && c.CoA > c.CoB) {
			c.A, c.B = c.B, c.A
			c.CoA, c.CoB = c.CoB, c.CoA
		}
	} else {
		c.B = RelOperand{}
		c.CoB = 0
	}
	lane, changed := l.mustSetLane.insert(c)
	return diffRelationLane{lane}, changed
}

func relOperandLess(a, b RelOperand) bool {
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	return a.Kind < b.Kind
}

// clearMatching drops every constraint whose A, B, or C operand (or the array
// behind a length node) matches, used when a path is mutated.
func (l diffRelationLane) clearMatching(match func(pathaddr.StateKey) bool) (diffRelationLane, bool) {
	if l.bottom || len(l.values) == 0 {
		return l, false
	}
	kept := make(map[RelConstraint]struct{}, len(l.values))
	changed := false
	for c := range l.values {
		if relOperandMatches(c.A, match) ||
			(c.B.valid() && relOperandMatches(c.B, match)) ||
			relOperandMatches(c.C, match) {
			changed = true
			continue
		}
		kept[c] = struct{}{}
	}
	if !changed {
		return l, false
	}
	return diffRelationLane{mustSetLaneFromLift(lift.MustSetValues(kept))}, true
}

func relOperandMatches(operand RelOperand, match func(pathaddr.StateKey) bool) bool {
	return operand.valid() && match(operand.Key)
}
