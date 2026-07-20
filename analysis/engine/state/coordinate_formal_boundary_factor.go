package state

import (
	"fmt"
	"sort"
)

// CoordinateFormalBoundaryFactorPlan is the sealed family-wise composition
// of existential boundary projection and formal-root publication for one
// coordinate lane. It consumes the already-factored formal family fibers and
// therefore never assembles an intermediate source LaneFactor.
type CoordinateFormalBoundaryFactorPlan struct {
	seal       *productDomainSeal
	projection CoordinateFormalPublicationProjection
	lane       ProductLane
	families   []coordinateFormalBoundaryFamilyPlan
}

type coordinateFormalBoundaryFamilyPlan struct {
	family CoordinateFamily
	slots  []CoordinateSlot
}

// CoordinateFormalBoundaryFamilyLayout exposes only the dense source slots
// needed to bind one registered family. Family algebra and target routing
// remain state-owned.
type CoordinateFormalBoundaryFamilyLayout struct {
	family CoordinateFamily
	slots  []CoordinateSlot
}

func (l CoordinateFormalBoundaryFamilyLayout) Family() CoordinateFamily { return l.family }
func (l CoordinateFormalBoundaryFamilyLayout) Slots() []CoordinateSlot {
	return append([]CoordinateSlot(nil), l.slots...)
}

// CoordinateFormalBoundaryFamilyOperands is one dense formal family fiber:
// Scalars aligns with the sealed layout slots. A zero scalar is the canonical
// omitted/default fiber, not an absent operand.
type CoordinateFormalBoundaryFamilyOperands struct {
	Skeleton CoordinateFamilySkeleton
	Scalars  []CoordinateScalarFactor
}

// SealCoordinateFormalBoundaryFactorPlan freezes the registration-driven
// family traversal for one coordinate lane. Selection and rekey authority are
// inherited from the already-sealed point publication projection.
func (d ProductDomain) SealCoordinateFormalBoundaryFactorPlan(
	projection CoordinateFormalPublicationProjection,
	lane ProductLane,
) (CoordinateFormalBoundaryFactorPlan, error) {
	if !d.OwnsCoordinateFormalPublicationProjection(projection) {
		return CoordinateFormalBoundaryFactorPlan{}, fmt.Errorf("%w: formal boundary factor projection", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateLane(lane)
	if err != nil || len(runtime.coordinates) == 0 || lane.slotFactored {
		return CoordinateFormalBoundaryFactorPlan{}, fmt.Errorf("%w: formal boundary factor requires a coordinate lane", ErrInvalidLaneFactor)
	}
	out := CoordinateFormalBoundaryFactorPlan{
		seal: d.seal, projection: projection, lane: lane,
		families: make([]coordinateFormalBoundaryFamilyPlan, len(runtime.coordinates)),
	}
	for index, coordinate := range runtime.coordinates {
		slots, slotsErr := projection.selection.coordinates.familySlots(coordinate.family)
		if slotsErr != nil {
			return CoordinateFormalBoundaryFactorPlan{}, slotsErr
		}
		out.families[index] = coordinateFormalBoundaryFamilyPlan{
			family: coordinate.family, slots: append([]CoordinateSlot(nil), slots...),
		}
	}
	return out, nil
}

func (p CoordinateFormalBoundaryFactorPlan) validFor(d ProductDomain) bool {
	return d.Valid() && p.seal == d.seal && p.lane.seal == d.seal &&
		d.OwnsCoordinateFormalPublicationProjection(p.projection) && len(p.families) != 0
}

func (p CoordinateFormalBoundaryFactorPlan) FamilyLayouts() []CoordinateFormalBoundaryFamilyLayout {
	out := make([]CoordinateFormalBoundaryFamilyLayout, len(p.families))
	for index, family := range p.families {
		out[index] = CoordinateFormalBoundaryFamilyLayout{
			family: family.family, slots: append([]CoordinateSlot(nil), family.slots...),
		}
	}
	return out
}

// ApplyCoordinateFormalBoundaryFactorPlan performs Select→Project→formal
// rekey as one family pass and composes the destination lane exactly once.
func (d ProductDomain) ApplyCoordinateFormalBoundaryFactorPlan(
	plan CoordinateFormalBoundaryFactorPlan,
	operands []CoordinateFormalBoundaryFamilyOperands,
) (LaneFactor, error) {
	if !plan.validFor(d) || len(operands) != len(plan.families) {
		return LaneFactor{}, fmt.Errorf("%w: formal boundary factor operands", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateLane(plan.lane)
	if err != nil || len(runtime.coordinates) != len(plan.families) {
		return LaneFactor{}, fmt.Errorf("%w: formal boundary factor lane", ErrInvalidLaneFactor)
	}
	skeletons := make([]CoordinateFamilySkeleton, len(plan.families))
	scalars := make([][]CoordinateScalarFactor, len(plan.families))
	projectCtx := boundaryProjectContext{
		reg: d.reg, keys: plan.projection.selection.keys,
		closure: plan.projection.selection.closure,
	}
	for familyIndex, familyPlan := range plan.families {
		coordinate := &runtime.coordinates[familyIndex]
		operand := operands[familyIndex]
		if coordinate.family != familyPlan.family || operand.Skeleton.family != familyPlan.family ||
			operand.Skeleton.keys != plan.projection.inverse.from || len(operand.Scalars) != len(familyPlan.slots) {
			return LaneFactor{}, fmt.Errorf("%w: formal boundary family %d operands", ErrInvalidLaneFactor, familyIndex)
		}
		if err := d.validateCoordinateSkeletonFor(coordinate, operand.Skeleton, plan.projection.inverse.from); err != nil {
			return LaneFactor{}, err
		}
		admitted := make([]coordinateKeyPayload, len(familyPlan.slots))
		for index, slot := range familyPlan.slots {
			if err := d.validateCoordinateSlotFor(coordinate, slot, plan.projection.inverse.from); err != nil {
				return LaneFactor{}, err
			}
			admitted[index] = slot.key
		}
		selectedSkeleton, post, ok := coordinate.ops.sealSkeletonInventory(
			operand.Skeleton.payload, admitted, plan.projection.inverse.from,
		)
		if !ok || selectedSkeleton == nil || len(post) != 0 {
			return LaneFactor{}, fmt.Errorf("%w: formal boundary family %q selection", ErrInvalidLaneFactor, familyPlan.family.id)
		}
		projectedSkeleton, ok := coordinate.boundary.projectSkeleton(&projectCtx, selectedSkeleton)
		if !ok || projectedSkeleton == nil {
			return LaneFactor{}, fmt.Errorf("state: formal boundary skeleton projection failed in family %q", familyPlan.family.id)
		}
		targetSkeleton, ok := coordinate.ops.formalRekey.skeleton(projectedSkeleton, plan.projection.inverse)
		if !ok || targetSkeleton == nil {
			return LaneFactor{}, fmt.Errorf("state: formal boundary skeleton rekey failed in family %q", familyPlan.family.id)
		}
		skeletons[familyIndex] = CoordinateFamilySkeleton{
			family: coordinate.family, keys: plan.projection.inverse.to, payload: targetSkeleton,
		}
		for sourceIndex, slot := range familyPlan.slots {
			source := operand.Scalars[sourceIndex]
			support := coordinate.ops.scalarSupport(selectedSkeleton, slot.key)
			if !support.valid() {
				return LaneFactor{}, fmt.Errorf("%w: formal boundary source support", ErrInvalidLaneFactor)
			}
			if source.payload == nil {
				if support == CoordinateScalarRequired {
					return LaneFactor{}, fmt.Errorf("%w: required formal boundary scalar is omitted", ErrIncompleteLaneFactors)
				}
				continue
			}
			if support == CoordinateScalarForbidden {
				return LaneFactor{}, fmt.Errorf("%w: forbidden formal boundary scalar is present", ErrInvalidLaneFactor)
			}
			if source.slot.family != coordinate.family || source.slot.keys != plan.projection.inverse.from ||
				!coordinate.ops.keyEqual(source.slot.key, slot.key) || !coordinate.ops.scalarValid(slot.key, source.payload) {
				return LaneFactor{}, fmt.Errorf("%w: formal boundary source scalar %d", ErrInvalidLaneFactor, sourceIndex)
			}
			if support == CoordinateScalarOptional {
				defaultScalar, defaultErr := coordinate.ops.defaultScalar(selectedSkeleton, slot.key)
				if defaultErr != nil || defaultScalar == nil {
					return LaneFactor{}, fmt.Errorf("%w: formal boundary source default", ErrInvalidLaneFactor)
				}
				if coordinate.ops.scalarEqual(source.payload, defaultScalar) {
					continue
				}
			}
			projectedKey, keep, valid := coordinate.boundary.projectKey(&projectCtx, slot.key)
			if !valid {
				return LaneFactor{}, fmt.Errorf("state: formal boundary key projection failed in family %q", familyPlan.family.id)
			}
			if !keep {
				continue
			}
			projectedScalar, valid := coordinate.boundary.projectScalar(&projectCtx, projectedKey, source.payload)
			if !valid || projectedScalar == nil {
				return LaneFactor{}, fmt.Errorf("state: formal boundary scalar projection failed in family %q", familyPlan.family.id)
			}
			targetKey, mapped := coordinate.ops.formalRekey.key(projectedKey, plan.projection.inverse)
			if !mapped || targetKey == nil || !coordinate.ops.keyValid(targetKey, plan.projection.inverse.to) {
				return LaneFactor{}, fmt.Errorf("state: formal boundary key rekey failed in family %q", familyPlan.family.id)
			}
			targetScalar, imported := coordinate.ops.importScalar(projectedScalar)
			if !imported || targetScalar == nil || !coordinate.ops.scalarValid(targetKey, targetScalar) {
				return LaneFactor{}, fmt.Errorf("state: formal boundary scalar rekey failed in family %q", familyPlan.family.id)
			}
			factor := CoordinateScalarFactor{
				slot:    CoordinateSlot{family: coordinate.family, keys: plan.projection.inverse.to, key: targetKey},
				payload: targetScalar,
			}
			omitted, omitErr := d.CoordinateScalarIsOmitted(skeletons[familyIndex], factor)
			if omitErr != nil {
				return LaneFactor{}, omitErr
			}
			if !omitted {
				scalars[familyIndex] = append(scalars[familyIndex], factor)
			}
		}
		sort.Slice(scalars[familyIndex], func(left, right int) bool {
			return coordinate.ops.keyLess(
				scalars[familyIndex][left].slot.key,
				scalars[familyIndex][right].slot.key,
				plan.projection.inverse.to,
			)
		})
	}
	return d.ComposeCoordinateFamilies(plan.lane, plan.projection.inverse.to, skeletons, scalars)
}
