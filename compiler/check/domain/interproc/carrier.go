package interproc

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// carrier.go is the seam between the interproc captured/constructor/container
// carriers, which store interned product.AbstractValue, and the rich per-element
// merge helpers (mergeInterprocValueType, joinCapturedType, container/constructor
// widening) that operate on typ.Type. A carrier slot projects in for the helpers
// (ProjectValue) and the helper result lifts back out (FromType), so the merge
// precision is unchanged and only the stored representation is value-domain.

// projectCarrier projects an interned carrier slot to its structural type. The
// zero AbstractValue (an absent slot) projects to nil so the typ.Type-based
// helpers see the same nil-slot signal they did before the carrier flip.
func projectCarrier(av product.AbstractValue) typ.Type {
	if av.IsZero() {
		return nil
	}
	return av.ProjectValue()
}

// liftCarrier lifts a merge-helper result back onto the carrier. A nil type lifts
// to the zero AbstractValue so an absent result survives the round-trip.
func liftCarrier(t typ.Type) product.AbstractValue {
	if t == nil {
		return product.AbstractValue{}
	}
	return product.FromType(t)
}
