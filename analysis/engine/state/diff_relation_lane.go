package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// DiffConstraint records difference-logic evidence proven on all paths reaching
// this state: value(Hi) - value(Lo) <= C. An array length operand uses
// LengthRelKey(arrayKey) so guards like i < j, i + 1 <= #xs, and #a == #b are
// all captured as constraints between value and length nodes. The lane is a
// must-set: join is intersection, so it carries only facts proven on every
// incoming edge and converges without weight widening.
type DiffConstraint struct {
	Hi pathdom.PathKey
	Lo pathdom.PathKey
	C  int64
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
	mustSetLane[DiffConstraint]
}

func diffRelationDomain() lattice.Lattice[diffRelationLane] {
	return wrapDomain(lift.MustSet[DiffConstraint](), diffRelationLaneFromMustSet, diffRelationLane.asMustSet)
}

func diffRelationLaneFromMustSet(l lift.MustSetLane[DiffConstraint]) diffRelationLane {
	return diffRelationLane{mustSetLaneFromLift(l)}
}

func (l diffRelationLane) reachable() diffRelationLane {
	return diffRelationLane{l.mustSetLane.reachable()}
}

func (l diffRelationLane) add(c DiffConstraint) (diffRelationLane, bool) {
	if c.Hi == "" || c.Lo == "" || c.Hi == c.Lo {
		return l, false
	}
	lane, changed := l.mustSetLane.insert(c)
	return diffRelationLane{lane}, changed
}

// clearMatching drops every constraint whose Hi or Lo operand (or the array
// behind a length node) matches, used when a path is mutated.
func (l diffRelationLane) clearMatching(match func(pathdom.PathKey) bool) (diffRelationLane, bool) {
	if l.bottom || len(l.values) == 0 {
		return l, false
	}
	kept := make(map[DiffConstraint]struct{}, len(l.values))
	changed := false
	for c := range l.values {
		if diffOperandMatches(c.Hi, match) || diffOperandMatches(c.Lo, match) {
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

func diffOperandMatches(operand pathdom.PathKey, match func(pathdom.PathKey) bool) bool {
	if match(operand) {
		return true
	}
	if array, ok := ArrayKeyOfLengthRel(operand); ok {
		return match(array)
	}
	return false
}
