package member

import (
	"github.com/wippyai/go-lua/analysis/schema"
)

// CandidateRef is the closed tagged choice of a candidate authority. Exactly
// one arm is stated; zero and both refuse.
//
// One type answers two questions at two altitudes: which rows a rule runs
// once per, and which directory a relation's rows are addressed through. They
// are the same question about the same authority, so a relation whose rows are
// keyed by an issued Program row states it the same way the rule does.
//
// AxisRelation is a domain-owned relation. The runtime resolves the dense
// candidate through that axis owner, from the mount and occurrence the rule is
// issued for.
//
// IssuedRow names an issuance relation instead. The rule's candidates are the
// Program rows that relation reaches from the occurrence row the rule is
// issued for, so the dense ordinal is the one issuance already computed while
// it owned both the source row and the occurrence identity. A relation entry
// already declares the space it reads and the space it targets, so the
// candidate row space is the relation's target and is not restated here.
//
// The two arms are alternatives, not layers: a rule whose candidates are
// Program rows has no Factor axis to resolve them through, and a rule whose
// candidates are axis rows has no issuance relation to reach them by.
type CandidateRef struct {
	AxisRelation RelationRef
	IssuedRow    schema.Key
}

// AxisRelationCandidate states the domain-owned arm.
func AxisRelationCandidate(relation RelationRef) CandidateRef {
	return CandidateRef{AxisRelation: relation}
}

// IssuedRowCandidate states the Program-owned arm by the issuance relation
// that reaches the candidate row from the issued occurrence.
func IssuedRowCandidate(relation schema.Key) CandidateRef {
	return CandidateRef{IssuedRow: relation}
}

// Issued reports which arm is stated. It is meaningful only on a declaration
// that is Available; a malformed value answers by its issued arm alone.
func (candidate CandidateRef) Issued() bool { return candidate.IssuedRow.Available() }

// Declared reports whether either arm is stated. It is the migration ratchet
// predicate: a Program with no candidate at all is the zero declaration.
func (candidate CandidateRef) Declared() bool {
	return candidate.AxisRelation.Declared() || candidate.IssuedRow.Available()
}

// Available reports whether exactly one arm is stated and complete.
func (candidate CandidateRef) Available() bool {
	if candidate.IssuedRow.Available() {
		return !candidate.AxisRelation.Declared()
	}
	return candidate.AxisRelation.Available()
}

// References returns the one upward reference of whichever arm is stated. The
// axis arm points at its member relation; the issued arm points at its
// issuance relation, so the same seal machinery that proves an axis relation
// exists proves the issuance relation exists.
func (candidate CandidateRef) References() schema.EntryReferences {
	if candidate.Issued() {
		return schema.EntryReferences{{Surface: schema.SurfaceKindIssuance, Key: candidate.IssuedRow}}
	}
	if !candidate.AxisRelation.Declared() {
		return nil
	}
	return schema.EntryReferences{candidate.AxisRelation.EntryReference()}
}
