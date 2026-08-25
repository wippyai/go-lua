package witness

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Inventory is the sole mutable-at-admission boundary consumed by
// Specialize.  Address and arrangement coordinates are resolved through their
// owning public APIs. Scope formulas and denominator evidence are supplied by
// the same mount snapshot; semantic authorities are explicit Specialize
// inputs and are never stored in Inventory.
//
// Implementations must keep all answers stable for the duration of one
// Specialize call.  Specialize snapshots every answer before returning.
type Inventory interface {
	address.Inventory
	arrangement.Inventory

	ScopeRegion(model.ScopeID) (Region, bool)
	ResolveDenominator(model.DenominatorRef) (DenominatorEvidence, bool)
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
	copyOf := append([]model.RowID(nil), rows...)
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
