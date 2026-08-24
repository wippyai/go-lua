package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// CandidateDecl is the closed tagged choice of a rule's candidate authority.
// Exactly one arm is stated; zero and both refuse.
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
type CandidateDecl struct {
	AxisRelation member.RelationRef
	IssuedRow    schema.Key
}

// AxisRelationCandidate states the domain-owned arm.
func AxisRelationCandidate(relation member.RelationRef) CandidateDecl {
	return CandidateDecl{AxisRelation: relation}
}

// IssuedRowCandidate states the Program-owned arm by the issuance relation
// that reaches the candidate row from the issued occurrence.
func IssuedRowCandidate(relation schema.Key) CandidateDecl {
	return CandidateDecl{IssuedRow: relation}
}

// Issued reports which arm is stated. It is meaningful only on a declaration
// that is Available; a malformed value answers by its issued arm alone.
func (candidate CandidateDecl) Issued() bool { return candidate.IssuedRow.Available() }

// Declared reports whether either arm is stated. It is the migration ratchet
// predicate: a Program with no candidate at all is the zero declaration.
func (candidate CandidateDecl) Declared() bool {
	return candidate.AxisRelation.Declared() || candidate.IssuedRow.Available()
}

// Available reports whether exactly one arm is stated and complete.
func (candidate CandidateDecl) Available() bool {
	if candidate.IssuedRow.Available() {
		return !candidate.AxisRelation.Declared()
	}
	return candidate.AxisRelation.Available()
}

// References returns the one upward reference of whichever arm is stated. The
// axis arm points at its member relation; the issued arm points at its
// issuance relation, so the same seal machinery that proves an axis relation
// exists proves the issuance relation exists.
func (candidate CandidateDecl) References() schema.EntryReferences {
	if candidate.Issued() {
		return schema.EntryReferences{{Surface: schema.SurfaceKindIssuance, Key: candidate.IssuedRow}}
	}
	if !candidate.AxisRelation.Declared() {
		return nil
	}
	return schema.EntryReferences{candidate.AxisRelation.EntryReference()}
}
