package summary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// AllocationRow is the typed value produced for one allocation child row.
//
// AllocationID is the Heap-issued allocation coordinate.  Fact and Evidence
// remain separate domain values because they have separate owner mathematics,
// but a row is admitted only when the two values describe the same reachable
// allocation.  This is a semantic operation value, not a second stored result
// representation.
type AllocationRow struct {
	AllocationID identity.ContentID
	Fact         placementdomain.Fact
	Evidence     placementdomain.AllocationEvidence
}

// ID returns the Heap-issued allocation coordinate carried by the row.
func (row AllocationRow) ID() identity.ContentID {
	if !row.Valid() {
		return identity.ContentID{}
	}
	return row.AllocationID
}

// Placement returns the canonical Fact only for a valid child row.
func (row AllocationRow) Placement() (placementdomain.Fact, bool) {
	if !row.Valid() {
		return placementdomain.Fact{}, false
	}
	return row.Fact, true
}

// EvidenceValue returns the complete Placement-owned evidence only for a
// valid child row.  The value is copied, so the owner product remains the
// authority for its optional/proof presence bits.
func (row AllocationRow) EvidenceValue() (placementdomain.AllocationEvidence, bool) {
	if !row.Valid() {
		return placementdomain.AllocationEvidence{}, false
	}
	return row.Evidence, true
}

// NewAllocationRow constructs the child product after checking its cross-plane
// coherence.  In particular, the row does not manufacture a default Fact or
// an all-absent evidence record for an unavailable input.
func NewAllocationRow(allocationID identity.ContentID, fact placementdomain.Fact, evidence placementdomain.AllocationEvidence) (AllocationRow, bool) {
	row := AllocationRow{AllocationID: allocationID, Fact: fact, Evidence: evidence}
	if !row.Valid() {
		return AllocationRow{}, false
	}
	return row, true
}

// Valid reports whether the row is a complete, owner-coherent child value.
// A reachable Fact must pass Placement's canonical cell authentication.  The
// evidence row must carry the same class and retain provenance and the exact
// Heap allocation identity; neither owner metadata nor producer proofs may be
// silently dropped at this boundary.
func (row AllocationRow) Valid() bool {
	if !row.AllocationID.Available() {
		return false
	}
	fact, factOK := placementdomain.AuthenticateFactCell(row.Fact, true, true)
	if !factOK || fact != row.Fact {
		return false
	}
	return AllocationEvidenceForFact(row.AllocationID, row.Fact, row.Evidence)
}

// Available is the ABI spelling used by owner-operation contracts.
func (row AllocationRow) Available() bool { return row.Valid() }

// AllocationEvidenceForFact checks the cross-owner product without changing
// the evidence value.  Evidence's optional fields and proof states are
// validated by Placement's canonical Evidence.Valid method, preserving
// EvidenceAbsent as distinct from an authenticated EvidenceUnknown.
func AllocationEvidenceForFact(allocationID identity.ContentID, fact placementdomain.Fact, evidence placementdomain.AllocationEvidence) bool {
	if !allocationID.Available() {
		return false
	}
	canonicalFact, factOK := placementdomain.AuthenticateFactCell(fact, true, true)
	if !factOK || canonicalFact != fact || !evidence.Valid() {
		return false
	}
	if !evidence.HasOwnerIdentity || evidence.OwnerIdentity != allocationID {
		return false
	}
	// A child row is complete at the allocation coordinate.  These two fields
	// are the Placement factor's paired canonical components; requiring both
	// prevents a class-only or stale-retain record from becoming a valid row.
	return evidence.HasClass && evidence.Class == fact.Class && evidence.RetainEscape == fact.RetainEscape
}

// ParentAnswer is the small parent answer product.  The parent row's emitted
// presence is the answer marker; PlacementSchemaID is the exact owner-issued
// schema identity that authenticated the child denominator. Presence is not
// duplicated as a payload field and cannot drift from the relation cell's
// canonical status.
type ParentAnswer struct {
	PlacementSchemaID identity.ContentID
}

// NewParentAnswer constructs an answer only from an exact Placement schema
// identity. It intentionally has no default answer; the binding's output
// presence is the only answer marker.
func NewParentAnswer(placementSchemaID identity.ContentID) (ParentAnswer, bool) {
	answer := ParentAnswer{PlacementSchemaID: placementSchemaID}
	if !answer.Valid() {
		return ParentAnswer{}, false
	}
	return answer, true
}

// Valid reports whether the parent carries an authenticated schema identity.
func (answer ParentAnswer) Valid() bool {
	return answer.PlacementSchemaID.Available()
}

// Available is the ABI spelling used by owner-operation contracts.
func (answer ParentAnswer) Available() bool { return answer.Valid() }

// SchemaID returns the exact Placement schema identity for a valid answer.
func (answer ParentAnswer) SchemaID() (identity.ContentID, bool) {
	if !answer.Valid() {
		return identity.ContentID{}, false
	}
	return answer.PlacementSchemaID, true
}
