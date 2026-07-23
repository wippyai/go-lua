package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// LengthFloorFactorPlan reuses the registered lower-bound law for a point
// write. There is no second LenFloor implementation for effects.
type LengthFloorFactorPlan struct {
	seal     *productDomainSeal
	mutation CoordinateBranchMutation
}

func (p LengthFloorFactorPlan) Valid() bool {
	return p.seal != nil && p.mutation.seal == p.seal && p.mutation.kind == coordinateBranchRelationIntegerBound
}

func (d ProductDomain) PrepareLengthFloorFactorPlan(keys *keyspace.KeySpace, path keyspace.Key, floor int64) (LengthFloorFactorPlan, error) {
	if floor <= 0 {
		return LengthFloorFactorPlan{}, fmt.Errorf("%w: non-positive length floor", ErrInvalidLaneFactor)
	}
	mutation, err := d.PrepareCoordinateBranchBound(CoordinateBoundLength, CoordinateBoundLower, keys, path, floor)
	if err != nil {
		return LengthFloorFactorPlan{}, err
	}
	return LengthFloorFactorPlan{seal: d.seal, mutation: mutation}, nil
}

func (d ProductDomain) LengthFloorFactorLane(plan LengthFloorFactorPlan) (ProductLane, error) {
	if !plan.Valid() || plan.seal != d.seal {
		return ProductLane{}, ErrInvalidLaneFactor
	}
	return plan.mutation.family.Lane(), nil
}

// LengthFloorFactorCoordinateWrites reports the one registered coordinate
// owned by a sealed length-floor mutation. Formal topology sealing uses this
// before execution so the later factor write cannot escape its fiber slice.
func (d ProductDomain) LengthFloorFactorCoordinateWrites(plan LengthFloorFactorPlan) ([]CoordinateSlot, error) {
	if !plan.Valid() || plan.seal != d.seal {
		return nil, ErrInvalidLaneFactor
	}
	return []CoordinateSlot{plan.mutation.slot}, nil
}

func (d ProductDomain) ApplyLengthFloorFactor(plan LengthFloorFactorPlan, current LaneFactor) (LaneFactor, error) {
	lane, err := d.LengthFloorFactorLane(plan)
	if err != nil || current.lane != lane {
		return LaneFactor{}, ErrInvalidLaneFactor
	}
	family := plan.mutation.family
	skeleton, scalars, err := d.DecomposeCoordinateFamily(current, family, plan.mutation.slot.keys)
	if err != nil {
		return LaneFactor{}, err
	}
	position, found, err := coordinateScalarPosition(d, scalars, plan.mutation.slot)
	if err != nil {
		return LaneFactor{}, err
	}
	var scalar CoordinateScalarFactor
	if found {
		scalar = scalars[position]
	} else {
		scalar, err = d.CoordinateDefault(skeleton, plan.mutation.slot)
		if err != nil {
			return LaneFactor{}, err
		}
	}
	nextSkeleton, nextScalar, err := d.ApplyCoordinateBranchMutation(plan.mutation, skeleton, scalar)
	if err != nil {
		return LaneFactor{}, err
	}
	return d.PatchCoordinateFamily(current, nextSkeleton, []CoordinateScalarFactor{nextScalar})
}

func (d ProductDomain) ApplyLengthFloor(plan LengthFloorFactorPlan, input State) (State, error) {
	lane, err := d.LengthFloorFactorLane(plan)
	if err != nil {
		return State{}, err
	}
	factors, err := d.DecomposeLanes(input, []ProductLane{lane})
	if err != nil {
		return State{}, err
	}
	next, err := d.ApplyLengthFloorFactor(plan, factors[0])
	if err != nil {
		return State{}, err
	}
	return d.PatchLaneFactors(input, []LaneFactor{next})
}

// ReadLengthFloorFactor observes one exact registered coordinate without
// reconstructing State. Missing/default coordinates report no evidence.
func (d ProductDomain) ReadLengthFloorFactor(factor LaneFactor, keys *keyspace.KeySpace, path keyspace.Key) (int64, bool, error) {
	family, ok := d.LenFloorCoordinateFamily()
	if !ok || factor.lane != family.Lane() {
		return 0, false, ErrInvalidLaneFactor
	}
	if keys == nil || !keys.Valid() || keys.FormatReadOnly(path) == "" {
		return 0, false, ErrInvalidLaneFactor
	}
	if _, _, err := d.validateCoordinateFamilyFactor(factor, family); err != nil {
		return 0, false, err
	}
	floor, present := typedLaneFactorValue[lenFloorLane](factor.payload).read(path)
	return floor, present, nil
}
