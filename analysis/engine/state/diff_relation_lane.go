package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// RelConstraint records relational evidence proven on all paths reaching this
// state: CoA*value(A) + CoB*value(B) - value(C) <= K. When B is empty it is an
// ordinary difference CoA*value(A) - value(C) <= K; when B is set it is a bounded
// affine relation between two positive operands (with positive coefficients) and
// one negative unit-coefficient operand. An array length operand uses
// LengthRelKey(arrayKey), so guards like i < j, i + 1 <= #xs, #a == #b,
// i + j <= #xs, and 2*i <= #xs are all captured as relations between value and
// length nodes. The lane is a must-set: join is intersection, so it carries only
// facts proven on every incoming edge and converges without weight widening.
type RelConstraint struct {
	CoA int64
	A   pathdom.PathKey
	CoB int64
	B   pathdom.PathKey
	C   pathdom.PathKey
	K   int64
}

// lengthRelPrefix marks a length node in the difference graph. It is not a valid
// structural path-key spelling, so it never collides with a real value path.
const lengthRelPrefix = "\x00len:"

// LengthRelKey returns the difference-graph node standing for len(arrayKey).
func LengthRelKey(arrayKey pathdom.PathKey) pathdom.PathKey {
	if arrayKey == "" {
		return ""
	}
	return lengthRelPrefix + arrayKey
}

// ArrayKeyOfLengthRel returns the array key a length node was derived from, and
// whether key is a length node.
func ArrayKeyOfLengthRel(key pathdom.PathKey) (pathdom.PathKey, bool) {
	s := string(key)
	if len(s) < len(lengthRelPrefix) || s[:len(lengthRelPrefix)] != lengthRelPrefix {
		return "", false
	}
	return pathdom.PathKey(s[len(lengthRelPrefix):]), true
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
	if c.A == "" || c.C == "" || c.A == c.C {
		return l, false
	}
	if c.B != "" {
		if c.A > c.B || (c.A == c.B && c.CoA > c.CoB) {
			c.A, c.B = c.B, c.A
			c.CoA, c.CoB = c.CoB, c.CoA
		}
	}
	lane, changed := l.mustSetLane.insert(c)
	return diffRelationLane{lane}, changed
}

// clearMatching drops every constraint whose A, B, or C operand (or the array
// behind a length node) matches, used when a path is mutated.
func (l diffRelationLane) clearMatching(match func(pathdom.PathKey) bool) (diffRelationLane, bool) {
	if l.bottom || len(l.values) == 0 {
		return l, false
	}
	kept := make(map[RelConstraint]struct{}, len(l.values))
	changed := false
	for c := range l.values {
		if relOperandMatches(c.A, match) ||
			(c.B != "" && relOperandMatches(c.B, match)) ||
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

func relOperandMatches(operand pathdom.PathKey, match func(pathdom.PathKey) bool) bool {
	if match(operand) {
		return true
	}
	if array, ok := ArrayKeyOfLengthRel(operand); ok {
		return match(array)
	}
	return false
}
