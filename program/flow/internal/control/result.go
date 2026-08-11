// Package control proves lexical control structure over the canonical
// authored Program relations. It deliberately does not construct a CFG,
// Outcomes, reachability, recurrence, or domain facts.
package control

import "github.com/wippyai/go-lua/program/keyspace"

// Shape is the transient lexical-control proof consumed by Outcome sealing.
// Direct control rows remain owned by authored Flow; Shape retains only the
// three derived selections that cannot be recovered without repeating the
// lexical proof.
type Shape struct {
	// Provenance is a scalar fence for all four sealed-owner identities:
	// Source, authored Flow, Static, and Module. Shape retains no owner
	// authority; downstream assembly must match all four, and any unavailable
	// identity makes these rows unusable.
	sourceID keyspace.ContentID
	flowID   keyspace.ContentID
	staticID keyspace.ContentID
	moduleID keyspace.ContentID

	labelBody      []keyspace.Term
	breakLoop      []keyspace.Term
	gotoTargetBody []keyspace.Term
}

// Matches reports whether s was sealed for the exact Source, authored Flow,
// Static, and Module identities supplied by final assembly. Unavailable
// identities never match, including a malformed Shape with plausible
// projection slices.
func Matches(s *Shape, sourceID, flowID, staticID, moduleID keyspace.ContentID) bool {
	return s != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		s.sourceID == sourceID && s.flowID == flowID && s.staticID == staticID && s.moduleID == moduleID
}

func (s *Shape) available() bool {
	return s != nil && s.sourceID.Available() && s.flowID.Available() && s.staticID.Available() && s.moduleID.Available()
}

// LabelBody returns the exact Body containing label.
func (s *Shape) LabelBody(label keyspace.Term) (keyspace.Term, bool) {
	if !s.available() {
		return 0, false
	}
	return shapeTerm(s.labelBody, label, keyspace.FamilyLabel)
}

// BreakLoop returns the nearest lexical Loop selected by breakTerm.
func (s *Shape) BreakLoop(breakTerm keyspace.Term) (keyspace.Term, bool) {
	if !s.available() {
		return 0, false
	}
	return shapeTerm(s.breakLoop, breakTerm, keyspace.FamilyBreak)
}

// GotoTargetBody returns the Body containing the accepted Goto target Label.
func (s *Shape) GotoTargetBody(gotoTerm keyspace.Term) (keyspace.Term, bool) {
	if !s.available() {
		return 0, false
	}
	return shapeTerm(s.gotoTargetBody, gotoTerm, keyspace.FamilyGoto)
}

func shapeTerm(rows []keyspace.Term, term keyspace.Term, family keyspace.Family) (keyspace.Term, bool) {
	if keyspace.TermFamily(term) != family {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return 0, false
	}
	if uint64(ordinal) >= uint64(len(rows)) || rows[ordinal] == 0 {
		return 0, false
	}
	return rows[ordinal], true
}
