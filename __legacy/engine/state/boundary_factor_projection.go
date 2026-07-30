package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

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
	projected := LaneFactor{lane: runtime.lane, payload: payload}
	if selection.exactCoordinates && len(runtime.coordinates) != 0 {
		if err := d.requireProjectedCoordinateCoverage(factor, projected, selection.keys); err != nil {
			return LaneFactor{}, err
		}
	}
	return projected, nil
}

// requireProjectedCoordinateCoverage is the exact-selection backstop for a
// dependent lane.  Once a caller selects a concrete coordinate, registered
// boundary projection may transform its payload but cannot silently erase its
// component.  Selection-side support closure makes the error actionable;
// without it a later read would observe an invented absence.
func (d ProductDomain) requireProjectedCoordinateCoverage(source, projected LaneFactor, keys *keyspace.KeySpace) error {
	runtime, err := d.validateFactor(source)
	if err != nil || projected.lane != runtime.lane || keys == nil || !keys.Valid() {
		return fmt.Errorf("%w: boundary coordinate coverage", ErrInvalidLaneFactor)
	}
	for _, coordinate := range runtime.coordinates {
		_, sourceScalars, sourceErr := d.DecomposeCoordinateFamily(source, coordinate.family, keys)
		_, projectedScalars, projectedErr := d.DecomposeCoordinateFamily(projected, coordinate.family, keys)
		if sourceErr != nil || projectedErr != nil {
			return fmt.Errorf("%w: boundary coordinate coverage in family %q", ErrInvalidLaneFactor, coordinate.family.id)
		}
		for _, scalar := range sourceScalars {
			_, present, findErr := coordinateShapeScalarIndex(d, projectedScalars, scalar.slot)
			if findErr != nil {
				return fmt.Errorf("%w: boundary coordinate coverage in family %q", ErrInvalidLaneFactor, coordinate.family.id)
			}
			if !present {
				return fmt.Errorf("%w: boundary projection in family %q omitted selected coordinate", ErrIncompleteLaneFactors, coordinate.family.id)
			}
		}
	}
	return nil
}
