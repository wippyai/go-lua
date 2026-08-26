package witness

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Inventory is the sole mutable-at-admission boundary consumed by
// Specialize.  Address and arrangement coordinates are resolved through their
// owning public APIs. Denominator evidence is supplied by the same mount
// snapshot; scope formulas are sealed in certificate ScopeSchemas. Semantic
// authorities are explicit Specialize inputs and are never stored in
// Inventory.
//
// Implementations must keep all answers stable for the duration of one
// Specialize call.  Specialize snapshots every answer before returning.
type Inventory interface {
	address.Inventory
	arrangement.Inventory

	ResolveDenominator(model.DenominatorRef) (DenominatorEvidence, bool)
	// ResolveExpand supplies the complete owner-issued C→P vectors for each
	// dependent expression. Specialize freezes them before arrangement sees
	// the immutable evidence catalog; arrangement never calls this method.
	ResolveExpand(model.ExpandContract) ([]expand.Vector, bool)
}

// PartitionInventory is the additional cold-mount evidence surface required
// by a checked correlated Apply. It returns raw owner evidence, never a bound
// runtime witness: Specialize validates the exact population keyset and
// child subsets under its own fence before issuing binding.PartitionDirectory
// values. The CorrelationPartition itself carries the child ordinal, so two
// occurrences with equal denominators cannot share an inferred posting.
//
// Implementations must return a non-nil map for an authenticated empty
// population and must keep the snapshot stable for the duration of one
// Specialize call. Inventory remains intentionally unchanged so unrelated
// mounts do not acquire a correlation-specific obligation.
type PartitionInventory interface {
	ResolvePartition(certificate.CorrelationPartition) (map[model.RowID]DenominatorEvidence, bool)
}

// DenominatorEvidence is the immutable logical row/evidence snapshot used to
// issue one denominator witness. Rows are ordered by the inventory's logical
// key order; they are never physical slots. The constructor copies rows.
type DenominatorEvidence struct {
	rows     []model.RowID
	evidence identity.ContentID
}

// NewDenominatorEvidence freezes one ordered row vector and its owner-issued
// evidence identity. The denominator relation is checked by Specialize,
// because this value is intentionally reusable by a resolver before its key
// reference is known.
func NewDenominatorEvidence(rows []model.RowID, evidence identity.ContentID) (DenominatorEvidence, bool) {
	if rows == nil || !evidence.Available() {
		return DenominatorEvidence{}, false
	}
	// Preserve a non-nil empty slice. An authenticated empty denominator is a
	// real closed-world membership, distinct from unavailable evidence; using
	// append to a nil slice would erase that distinction and make the valid
	// empty view fail admission downstream.
	copyOf := make([]model.RowID, len(rows))
	copy(copyOf, rows)
	for index, row := range copyOf {
		if !row.Available() {
			return DenominatorEvidence{}, false
		}
		for _, prior := range copyOf[:index] {
			if prior == row {
				return DenominatorEvidence{}, false
			}
		}
	}
	return DenominatorEvidence{rows: copyOf, evidence: evidence}, true
}

// Available reports whether the row/evidence snapshot is complete.
func (value DenominatorEvidence) Available() bool {
	if value.rows == nil || !value.evidence.Available() {
		return false
	}
	for index, row := range value.rows {
		if !row.Available() {
			return false
		}
		for _, prior := range value.rows[:index] {
			if prior == row {
				return false
			}
		}
	}
	return true
}

// Rows returns a defensive copy of the logical denominator members.
func (value DenominatorEvidence) Rows() []model.RowID {
	if !value.Available() {
		return nil
	}
	copyOf := make([]model.RowID, len(value.rows))
	copy(copyOf, value.rows)
	return copyOf
}

// Evidence returns the immutable identity authenticating the row snapshot.
func (value DenominatorEvidence) Evidence() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.evidence
}
