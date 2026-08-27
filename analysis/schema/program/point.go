package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

// PointDecision is one decision identity a program point commits to. Its
// position is its ordinal in PointDecisionFamily and the parent point names
// the half-open span it references, so no point retains a slice header.
//
// The semantic identity and neutral atom are deliberately carried by this
// same row. The semantic identity is the exact Flow path used by equation
// joins; the atom is the owner-issued neutral region proposition used by the
// physical cofiber. Neither side derives the other at a later mount.
type PointDecision struct {
	id   identity.ContentID
	atom region.Atom
}

// NewPointDecision adopts the exact Flow semantic identity and its
// owner-issued neutral atom as one Program row. The two identities are kept
// independent: a valid owner may issue an atom identity distinct from the
// semantic path, and this constructor must preserve that distinction.
func NewPointDecision(id identity.ContentID, atom region.Atom) (PointDecision, bool) {
	row := PointDecision{id: id, atom: atom}
	return row, row.Available() && atom.Available()
}

func (row PointDecision) Available() bool { return row.id.Available() && row.atom.Available() }

func (row PointDecision) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

// Atom returns the exact owner-issued neutral proposition attached to this
// semantic decision row. It is not a guard atom and carries no physical
// coordinate meaning.
func (row PointDecision) Atom() region.Atom {
	if !row.Available() {
		return region.Atom{}
	}
	return row.atom
}

// Point is one program point's geometry. Its decisions are a span in
// PointDecisionFamily, preserving the canonical decision order while making
// this row flat and copy-safe.
type Point struct {
	id             identity.ContentID
	scope          identity.ContentID
	initial        bool
	decisionOffset uint32
	decisionCount  uint32
}

// NewPoint copies one canonical Point row and replaces its nested decision
// slice with a dense PointDecisionFamily span.
func NewPoint(id, scope identity.ContentID, initial bool, decisionOffset, decisionCount uint32) (Point, bool) {
	row := Point{id: id, scope: scope, initial: initial, decisionOffset: decisionOffset, decisionCount: decisionCount}
	return row, row.Available()
}

func (row Point) Available() bool {
	return row.id.Available() && row.scope.Available() && uint64(row.decisionOffset)+uint64(row.decisionCount) <= uint64(^uint32(0))
}

// ScopeID returns the exact Program point that owns this row's decision
// coordinate space. A base point owns itself; synthetic stage points retain
// the base identity instead of copying or re-deriving its decision vector.
func (row Point) ScopeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.scope
}

func (row Point) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Point) Initial() bool { return row.Available() && row.initial }

func (row Point) DecisionSpan() (offset, count uint32, ok bool) {
	return row.decisionOffset, row.decisionCount, row.Available()
}

func (row Point) DecisionCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.decisionCount)
}
