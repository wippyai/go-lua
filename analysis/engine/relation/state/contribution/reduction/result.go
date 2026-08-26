package reduction

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Aggregate is the derived value for one contribution target.
//
// A non-removal aggregate retains the exact destination CellToken issued by
// the mounted owner, the reduced owner ValueToken, its declared Presence, and
// the lineage join of the surviving producer rows.  Removal is deliberately a
// separate operation bit: a target with no surviving rows is sparse/undefined,
// not ProvenAbsent and not an invented zero value.
//
// The destination is not reconstructed from Target.Destination.  For a
// non-empty row set it is adopted from the retained row cell and checked for
// exact equality across all rows.  Empty target reduction requires the
// caller's owner-issued cell via ReduceAt; this keeps the reducer from
// manufacturing a CellToken.
type Aggregate struct {
	target      contribution.Target
	destination binding.CellToken
	value       binding.ValueToken
	presence    model.Presence
	lineage     model.LineageRef
	removal     bool
	sealed      bool
}

// Available reports whether the result is a complete derived projection.
// Sparse removal intentionally has no semantic value, Presence, or lineage;
// its target remains available so the caller can address the exact column and
// row without converting sparse absence into ProvenAbsent.
func (aggregate Aggregate) Available() bool {
	if !aggregate.sealed || !aggregate.target.Available() {
		return false
	}
	if aggregate.removal {
		return aggregate.destination.Available() && aggregate.destination.ValidFor(aggregate.destination.Fence()) && !aggregate.value.Available() && !aggregate.presence.Available() && !aggregate.lineage.Available()
	}
	return aggregate.destination.Available() && aggregate.value.Available() && (aggregate.presence.Is(model.Present) || aggregate.presence.Is(model.AuthenticatedOpaque)) && aggregate.lineage.Available()
}

// Target returns the exact schema output target.
func (aggregate Aggregate) Target() contribution.Target {
	if !aggregate.Available() {
		return contribution.Target{}
	}
	return aggregate.target
}

// Destination returns the exact retained owner-issued destination cell.  A
// sparse aggregate still retains the exact destination cell to remove.
func (aggregate Aggregate) Destination() (binding.CellToken, bool) {
	if !aggregate.Available() || !aggregate.destination.Available() {
		return binding.CellToken{}, false
	}
	return aggregate.destination, true
}

// Value returns the reduced owner-issued semantic value.  Sparse removal has
// no value by design.
func (aggregate Aggregate) Value() (binding.ValueToken, bool) {
	if !aggregate.Available() || aggregate.removal {
		return binding.ValueToken{}, false
	}
	return aggregate.value, true
}

// Presence returns the exact declared output status for a non-removal result.
// Sparse removal has no Presence; in particular it is not ProvenAbsent.
func (aggregate Aggregate) Presence() (model.Presence, bool) {
	if !aggregate.Available() || aggregate.removal {
		return model.Presence{}, false
	}
	return aggregate.presence, true
}

// Lineage returns the joined proof sidecar for a non-removal result.
func (aggregate Aggregate) Lineage() (model.LineageRef, bool) {
	if !aggregate.Available() || aggregate.removal {
		return model.LineageRef{}, false
	}
	return aggregate.lineage, true
}

// Removal reports that the target has no surviving contribution rows and the
// derived cell must be removed sparsely.
func (aggregate Aggregate) Removal() bool {
	return aggregate.Available() && aggregate.removal
}

// Sparse is the explicit sparse result spelling.  It is equivalent to
// Removal, but names the storage operation rather than the producer event.
func (aggregate Aggregate) Sparse() bool { return aggregate.Removal() }
