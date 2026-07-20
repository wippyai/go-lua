package state

import "fmt"

// ProjectBoundaryFactor applies one non-Values lane's registered boundary
// projection law under an already-closed factor selection. It is the
// projection-only counterpart of PrepareBoundaryFactorTransportPlan: no State
// is assembled, no rebase/apply occurs, and no product-lane inventory is
// scanned. The returned factor retains the exact registered lane identity.
func (d ProductDomain) ProjectBoundaryFactor(selection BoundaryFactorSelection, factor LaneFactor) (LaneFactor, error) {
	if !d.Valid() || !selection.valid() {
		return LaneFactor{}, fmt.Errorf("%w: boundary factor projection is unowned", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.slotFactored || runtime.ops.boundaryProject == nil {
		return LaneFactor{}, fmt.Errorf("%w: lane has no non-Values boundary projection", ErrInvalidLaneFactor)
	}
	if selection.exactCoordinates && len(runtime.coordinates) != 0 {
		factor, err = d.SelectCoordinateLaneFactor(factor, selection.coordinates)
		if err != nil {
			return LaneFactor{}, err
		}
	}
	ctx := boundaryProjectContext{reg: d.reg, keys: selection.keys, closure: selection.closure}
	payload, ok := runtime.ops.boundaryProject(&ctx, factor.payload)
	if !ok {
		return LaneFactor{}, fmt.Errorf("state: boundary factor projection failed in lane %q", runtime.lane.id)
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, nil
}
