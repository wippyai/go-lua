package value

import "github.com/wippyai/go-lua/domain/materialization"

// AllocationMembership is Value's closed classification of one owned Value
// relation against one exact AllocationResult. It deliberately says nothing
// about aliases, uniqueness, escape, freezing, lifetime, or placement.
type AllocationMembership uint8

const (
	AllocationMembershipInvalid AllocationMembership = iota
	MembershipRecent
	MembershipSummary
	MembershipMixedOrUnknown
)

func (membership AllocationMembership) Valid() bool {
	return membership == MembershipRecent || membership == MembershipSummary || membership == MembershipMixedOrUnknown
}

// ClassifyMembership classifies exactly one owner-fenced Value relation
// against this exact allocation receipt. Only a singleton rooted Recent or
// Summary atom for the same Heap key receives a positive class. Top, Bottom,
// a different root, or any multi-alternative relation is MixedOrUnknown.
//
// This is intentionally a pure Value operation. It receives no solver,
// coordinate, summary-vector, or runtime context authority and can therefore
// never itself authorize a placement decision.
func (result *AllocationResult) ClassifyMembership(fact Value) (AllocationMembership, bool) {
	if result == nil || result.schema == nil || !result.validFor(result.schema) || !result.schema.owns(fact) {
		return AllocationMembershipInvalid, false
	}
	issuedKeyID, issuedKeyOK := result.key.ContentID()
	canonical, canonicalOK := result.schema.AllocationResultFor(result.key)
	if !issuedKeyOK || result.keyID != issuedKeyID || !canonicalOK || canonical != result {
		return AllocationMembershipInvalid, false
	}
	if fact.top {
		return MembershipMixedOrUnknown, true
	}
	atoms, atomsOK := result.schema.Atoms(fact)
	if !atomsOK || len(atoms) != 1 {
		return MembershipMixedOrUnknown, true
	}
	reference, role, referenceOK := atoms[0].Reference()
	key, keyOK := reference.AllocationKey()
	if !referenceOK || !keyOK || key != result.key {
		return MembershipMixedOrUnknown, true
	}
	switch role {
	case materialization.Recent:
		return MembershipRecent, true
	case materialization.Summary:
		return MembershipSummary, true
	default:
		return MembershipMixedOrUnknown, true
	}
}
