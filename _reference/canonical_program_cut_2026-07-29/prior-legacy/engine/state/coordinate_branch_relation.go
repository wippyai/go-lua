package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// CoordinateBoundOperand is the semantic quantity constrained by a scalar
// branch bound. It is product-neutral: registered families decide whether
// they own a value or length bound.
type CoordinateBoundOperand uint8

const (
	CoordinateBoundValue CoordinateBoundOperand = iota + 1
	CoordinateBoundLength
)

// CoordinateBoundDirection names the monotone information order of a bound.
type CoordinateBoundDirection uint8

const (
	CoordinateBoundLower CoordinateBoundDirection = iota + 1
	CoordinateBoundUpper
)

type coordinateBranchRelationKind uint8

const (
	coordinateBranchRelationIndependent coordinateBranchRelationKind = iota + 1
	coordinateBranchRelationIntegerBound
	coordinateBranchRelationLinearConstraint
)

type coordinateBranchRelationOps struct {
	kind        coordinateBranchRelationKind
	operand     CoordinateBoundOperand
	direction   CoordinateBoundDirection
	boundKey    func(*keyspace.KeySpace, keyspace.Key) (coordinateKeyPayload, bool)
	boundApply  func(coordinateSkeletonPayload, coordinateKeyPayload, coordinateScalarPayload, int64) (coordinateSkeletonPayload, coordinateScalarPayload, bool)
	linearKey   func(*keyspace.KeySpace, RelConstraint) (coordinateKeyPayload, bool)
	linearApply func(coordinateSkeletonPayload, coordinateKeyPayload, coordinateScalarPayload, RelConstraint) (coordinateSkeletonPayload, coordinateScalarPayload, bool)
}

func noCoordinateBranchRelation() coordinateBranchRelationOps {
	return coordinateBranchRelationOps{kind: coordinateBranchRelationIndependent}
}

func coordinateIntegerBoundBranchRelation(
	operand CoordinateBoundOperand,
	direction CoordinateBoundDirection,
	key func(*keyspace.KeySpace, keyspace.Key) (coordinateKeyPayload, bool),
	apply func(coordinateSkeletonPayload, coordinateKeyPayload, coordinateScalarPayload, int64) (coordinateSkeletonPayload, coordinateScalarPayload, bool),
) coordinateBranchRelationOps {
	return coordinateBranchRelationOps{kind: coordinateBranchRelationIntegerBound, operand: operand, direction: direction, boundKey: key, boundApply: apply}
}

func coordinateLinearConstraintBranchRelation(
	key func(*keyspace.KeySpace, RelConstraint) (coordinateKeyPayload, bool),
	apply func(coordinateSkeletonPayload, coordinateKeyPayload, coordinateScalarPayload, RelConstraint) (coordinateSkeletonPayload, coordinateScalarPayload, bool),
) coordinateBranchRelationOps {
	return coordinateBranchRelationOps{kind: coordinateBranchRelationLinearConstraint, linearKey: key, linearApply: apply}
}

func coordinateBranchRelationOpsComplete(ops coordinateBranchRelationOps) bool {
	switch ops.kind {
	case coordinateBranchRelationIndependent:
		return ops.operand == 0 && ops.direction == 0 && ops.boundKey == nil && ops.boundApply == nil && ops.linearKey == nil && ops.linearApply == nil
	case coordinateBranchRelationIntegerBound:
		return (ops.operand == CoordinateBoundValue || ops.operand == CoordinateBoundLength) &&
			(ops.direction == CoordinateBoundLower || ops.direction == CoordinateBoundUpper) &&
			ops.boundKey != nil && ops.boundApply != nil && ops.linearKey == nil && ops.linearApply == nil
	case coordinateBranchRelationLinearConstraint:
		return ops.operand == 0 && ops.direction == 0 && ops.boundKey == nil && ops.boundApply == nil && ops.linearKey != nil && ops.linearApply != nil
	default:
		return false
	}
}

// CoordinateBranchMutation is one freeze-time selected registered coordinate
// operation. The family callback and exact slot are sealed once; leaf
// execution performs no family lookup, interface dispatch, or inventory scan.
type CoordinateBranchMutation struct {
	seal        *productDomainSeal
	family      CoordinateFamily
	slot        CoordinateSlot
	kind        coordinateBranchRelationKind
	bound       int64
	constraint  RelConstraint
	applyBound  func(coordinateSkeletonPayload, coordinateKeyPayload, coordinateScalarPayload, int64) (coordinateSkeletonPayload, coordinateScalarPayload, bool)
	applyLinear func(coordinateSkeletonPayload, coordinateKeyPayload, coordinateScalarPayload, RelConstraint) (coordinateSkeletonPayload, coordinateScalarPayload, bool)
}

func (m CoordinateBranchMutation) Slot() CoordinateSlot { return m.slot }

// CoordinateBranchMutationLane returns the unique registration-selected
// carrier for a prepared mutation.
func (d ProductDomain) CoordinateBranchMutationLane(mutation CoordinateBranchMutation) (ProductLane, error) {
	if !d.Valid() || mutation.seal != d.seal || mutation.family.seal != d.seal {
		return ProductLane{}, ErrInvalidLaneFactor
	}
	return mutation.family.Lane(), nil
}

// PrepareCoordinateBranchBound resolves exactly one registered owner. Missing
// and duplicate participants are admission failures, never silent no-ops.
func (d ProductDomain) PrepareCoordinateBranchBound(operand CoordinateBoundOperand, direction CoordinateBoundDirection, keys *keyspace.KeySpace, path keyspace.Key, bound int64) (CoordinateBranchMutation, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || path.Kind == keyspace.KindInvalid {
		return CoordinateBranchMutation{}, fmt.Errorf("%w: invalid branch bound", ErrInvalidLaneFactor)
	}
	var out CoordinateBranchMutation
	found := false
	for _, lane := range d.factorLanes {
		for _, family := range lane.coordinates {
			ops := family.ops.branchRelation
			if ops.kind != coordinateBranchRelationIntegerBound || ops.operand != operand || ops.direction != direction {
				continue
			}
			if found {
				return CoordinateBranchMutation{}, fmt.Errorf("%w: duplicate branch bound owner", ErrInvalidLaneFactor)
			}
			key, ok := ops.boundKey(keys, path)
			if !ok || key == nil || !family.ops.keyValid(key, keys) {
				return CoordinateBranchMutation{}, fmt.Errorf("%w: branch bound key", ErrInvalidLaneFactor)
			}
			out = CoordinateBranchMutation{
				seal: d.seal, family: family.family, slot: CoordinateSlot{family: family.family, keys: keys, key: key},
				kind: ops.kind, bound: bound, applyBound: ops.boundApply,
			}
			found = true
		}
	}
	if !found {
		return CoordinateBranchMutation{}, fmt.Errorf("%w: branch bound owner absent", ErrInvalidLaneFactor)
	}
	return out, nil
}

func (d ProductDomain) PrepareCoordinateBranchConstraint(keys *keyspace.KeySpace, constraint RelConstraint) (CoordinateBranchMutation, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return CoordinateBranchMutation{}, fmt.Errorf("%w: invalid branch constraint", ErrInvalidLaneFactor)
	}
	var out CoordinateBranchMutation
	found := false
	for _, lane := range d.factorLanes {
		for _, family := range lane.coordinates {
			ops := family.ops.branchRelation
			if ops.kind != coordinateBranchRelationLinearConstraint {
				continue
			}
			if found {
				return CoordinateBranchMutation{}, fmt.Errorf("%w: duplicate branch constraint owner", ErrInvalidLaneFactor)
			}
			key, ok := ops.linearKey(keys, constraint)
			if !ok || key == nil || !family.ops.keyValid(key, keys) {
				return CoordinateBranchMutation{}, fmt.Errorf("%w: branch constraint key", ErrInvalidLaneFactor)
			}
			out = CoordinateBranchMutation{
				seal: d.seal, family: family.family, slot: CoordinateSlot{family: family.family, keys: keys, key: key},
				kind: ops.kind, constraint: constraint, applyLinear: ops.linearApply,
			}
			found = true
		}
	}
	if !found {
		return CoordinateBranchMutation{}, fmt.Errorf("%w: branch constraint owner absent", ErrInvalidLaneFactor)
	}
	return out, nil
}

// ApplyCoordinateBranchMutation rewrites one exact scalar and its family
// skeleton through the freeze-selected registered law.
func (d ProductDomain) ApplyCoordinateBranchMutation(mutation CoordinateBranchMutation, skeleton CoordinateFamilySkeleton, current CoordinateScalarFactor) (CoordinateFamilySkeleton, CoordinateScalarFactor, error) {
	if !d.Valid() || mutation.seal != d.seal || mutation.family != skeleton.family || mutation.family != current.slot.family ||
		mutation.slot.family != mutation.family || skeleton.keys != mutation.slot.keys || current.slot.keys != mutation.slot.keys {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, fmt.Errorf("%w: foreign branch coordinate mutation", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || d.validateCoordinateFactorFor(coordinate, current, skeleton.keys) != nil {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	equal, err := d.CoordinateSlotEqual(mutation.slot, current.slot)
	if err != nil || !equal {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	var nextSkeleton coordinateSkeletonPayload
	var nextScalar coordinateScalarPayload
	var ok bool
	switch mutation.kind {
	case coordinateBranchRelationIntegerBound:
		nextSkeleton, nextScalar, ok = mutation.applyBound(skeleton.payload, mutation.slot.key, current.payload, mutation.bound)
	case coordinateBranchRelationLinearConstraint:
		nextSkeleton, nextScalar, ok = mutation.applyLinear(skeleton.payload, mutation.slot.key, current.payload, mutation.constraint)
	}
	if !ok || nextSkeleton == nil || nextScalar == nil || !coordinate.ops.scalarValid(current.slot.key, nextScalar) {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, fmt.Errorf("%w: branch coordinate mutation rejected by family %q", ErrInvalidLaneFactor, coordinate.family.id)
	}
	return CoordinateFamilySkeleton{family: mutation.family, keys: skeleton.keys, payload: nextSkeleton}, CoordinateScalarFactor{slot: current.slot, payload: nextScalar}, nil
}

// ApplyCoordinateBranchMutationFactor applies the registered scalar law to
// one complete lane factor. It is the carrier-neutral counterpart of the
// concrete numeric/relational State writers.
func (d ProductDomain) ApplyCoordinateBranchMutationFactor(mutation CoordinateBranchMutation, current LaneFactor) (LaneFactor, error) {
	lane, err := d.CoordinateBranchMutationLane(mutation)
	if err != nil || current.Lane() != lane {
		return LaneFactor{}, ErrInvalidLaneFactor
	}
	skeleton, scalars, err := d.DecomposeCoordinateFamily(current, mutation.family, mutation.slot.keys)
	if err != nil {
		return LaneFactor{}, err
	}
	position, found, err := coordinateScalarPosition(d, scalars, mutation.slot)
	if err != nil {
		return LaneFactor{}, err
	}
	var scalar CoordinateScalarFactor
	if found {
		scalar = scalars[position]
	} else {
		scalar, err = d.CoordinateDefault(skeleton, mutation.slot)
		if err != nil {
			return LaneFactor{}, err
		}
	}
	nextSkeleton, nextScalar, err := d.ApplyCoordinateBranchMutation(mutation, skeleton, scalar)
	if err != nil {
		return LaneFactor{}, err
	}
	return d.PatchCoordinateFamily(current, nextSkeleton, []CoordinateScalarFactor{nextScalar})
}
