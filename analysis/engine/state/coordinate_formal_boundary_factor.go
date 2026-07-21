package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
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

// FormalBoundarySourceSlot returns the frozen formal slot whose registered
// boundary image is target.  The scan deliberately follows the sealed family
// layouts instead of building a second execution map: a target coordinate may
// be absent after boundary selection, and an ambiguous image is not a lawful
// demand translation.
func (d ProductDomain) FormalBoundarySourceSlot(
	plan CoordinateFormalBoundaryFactorPlan,
	target CoordinateSlot,
) (CoordinateSlot, bool, error) {
	if !plan.validFor(d) || target.keys != plan.projection.inverse.to {
		return CoordinateSlot{}, false, fmt.Errorf("%w: formal boundary target slot", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(target.family)
	if err != nil || d.validateCoordinateSlotFor(coordinate, target, target.keys) != nil {
		return CoordinateSlot{}, false, fmt.Errorf("%w: formal boundary target slot", ErrInvalidLaneFactor)
	}
	projectCtx := boundaryProjectContext{
		reg: d.reg, keys: plan.projection.selection.keys,
		closure: plan.projection.selection.closure,
	}
	var source CoordinateSlot
	found := false
	for _, familyPlan := range plan.families {
		if familyPlan.family != target.family {
			continue
		}
		for _, candidate := range familyPlan.slots {
			projected, keep, valid := coordinate.boundary.projectKey(&projectCtx, candidate.key)
			if !valid {
				return CoordinateSlot{}, false, fmt.Errorf("state: formal boundary key projection failed in family %q", target.family.id)
			}
			if !keep {
				continue
			}
			mapped, ok := coordinate.ops.formalRekey.key(projected, plan.projection.inverse)
			if !ok || mapped == nil || !coordinate.ops.keyValid(mapped, target.keys) {
				return CoordinateSlot{}, false, fmt.Errorf("state: formal boundary key rekey failed in family %q", target.family.id)
			}
			match, equalErr := d.CoordinateSlotEqual(CoordinateSlot{family: target.family, keys: target.keys, key: mapped}, target)
			if equalErr != nil {
				return CoordinateSlot{}, false, equalErr
			}
			if !match {
				continue
			}
			if found {
				return CoordinateSlot{}, false, fmt.Errorf("%w: ambiguous formal boundary target slot", ErrInvalidLaneFactor)
			}
			source, found = candidate, true
		}
	}
	if !found {
		// Dynamic path evidence can name a point-local identity image that was
		// not present in the static boundary selection.  Its source spelling is
		// still determined by the same sealed inverse root law; it is not an
		// invitation to manufacture a coordinate or scan a runtime map.
		inverse := plan.projection.inverse
		reverse := CoordinateFormalRootRekey{
			seal: inverse.seal, targetOwner: inverse.sourceOwner, formalTarget: inverse.formalSource,
			formalSource: inverse.formalTarget, sourceOwner: inverse.targetOwner,
			from: inverse.to, to: inverse.from,
			roots:         append(boundaryPathMap(nil), inverse.inverseRoots...),
			inverseRoots:  append(boundaryPathMap(nil), inverse.roots...),
			rootIndex:     make(map[keyspace.Key]keyspace.Key, len(inverse.roots)),
			resolverIndex: make(map[keyspace.Key]keyspace.Key, len(inverse.roots)),
		}
		for _, binding := range inverse.roots {
			if _, resolver := inverse.resolverIndex[binding.from]; resolver {
				reverse.resolverIndex[binding.to] = binding.from
			} else {
				reverse.rootIndex[binding.to] = binding.from
			}
		}
		mapped, ok := coordinate.ops.formalRekey.key(target.key, reverse)
		if !ok || mapped == nil || !coordinate.ops.keyValid(mapped, reverse.to) {
			return CoordinateSlot{}, false, nil
		}
		return CoordinateSlot{family: target.family, keys: reverse.to, key: mapped}, true, nil
	}
	return source, true, nil
}

// RekeyFormalBoundaryCoordinateEvidence translates one selected formal family
// answer through its frozen boundary image.  It is intentionally narrower than
// full lane composition: cursor rounds own only the demanded scalars, while
// the frozen plan remains the sole authority for selection, projection, and
// rekeying.
func (d ProductDomain) RekeyFormalBoundaryCoordinateEvidence(
	plan CoordinateFormalBoundaryFactorPlan,
	family CoordinateFamily,
	skeleton CoordinateFamilySkeleton,
	scalars []CoordinateScalarFactor,
) (CoordinateFamilySkeleton, []CoordinateScalarFactor, error) {
	if !plan.validFor(d) || skeleton.family != family || skeleton.keys != plan.projection.inverse.from {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary coordinate evidence", ErrInvalidLaneFactor)
	}
	var familyPlan *coordinateFormalBoundaryFamilyPlan
	for index := range plan.families {
		if plan.families[index].family == family {
			familyPlan = &plan.families[index]
			break
		}
	}
	if familyPlan == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary coordinate family", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || d.validateCoordinateSkeletonFor(coordinate, skeleton, skeleton.keys) != nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary coordinate skeleton", ErrInvalidLaneFactor)
	}
	admitted := make([]coordinateKeyPayload, len(familyPlan.slots))
	for index, slot := range familyPlan.slots {
		admitted[index] = slot.key
	}
	selected, post, ok := coordinate.ops.sealSkeletonInventory(skeleton.payload, admitted, skeleton.keys)
	if !ok || selected == nil || len(post) != 0 {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary coordinate selection", ErrInvalidLaneFactor)
	}
	projectCtx := boundaryProjectContext{reg: d.reg, keys: plan.projection.selection.keys, closure: plan.projection.selection.closure}
	projectedSkeleton, ok := coordinate.boundary.projectSkeleton(&projectCtx, selected)
	if !ok || projectedSkeleton == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: formal boundary skeleton projection failed in family %q", family.id)
	}
	targetSkeletonPayload, ok := coordinate.ops.formalRekey.skeleton(projectedSkeleton, plan.projection.inverse)
	if !ok || targetSkeletonPayload == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: formal boundary skeleton rekey failed in family %q", family.id)
	}
	targetSkeleton := CoordinateFamilySkeleton{family: family, keys: plan.projection.inverse.to, payload: targetSkeletonPayload}
	out := make([]CoordinateScalarFactor, len(scalars))
	seen := make([]bool, len(familyPlan.slots))
	selectedSources := true
	for _, scalar := range scalars {
		present := false
		for _, slot := range familyPlan.slots {
			equal, equalErr := d.CoordinateSlotEqual(slot, scalar.slot)
			if equalErr != nil {
				return CoordinateFamilySkeleton{}, nil, equalErr
			}
			present = present || equal
		}
		selectedSources = selectedSources && present
	}
	if !selectedSources {
		targetSkeleton, skeletonErr := d.RekeyCoordinateSkeletonFormal(plan.projection.inverse, skeleton)
		if skeletonErr != nil {
			return CoordinateFamilySkeleton{}, nil, skeletonErr
		}
		out := make([]CoordinateScalarFactor, len(scalars))
		for index, scalar := range scalars {
			rekeyed, scalarErr := d.RekeyCoordinateScalarFormal(plan.projection.inverse, scalar)
			if scalarErr != nil {
				return CoordinateFamilySkeleton{}, nil, scalarErr
			}
			out[index] = rekeyed
		}
		return targetSkeleton, out, nil
	}
	for scalarIndex, scalar := range scalars {
		if scalar.slot.family != family || scalar.slot.keys != skeleton.keys ||
			d.validateCoordinateFactorFor(coordinate, scalar, skeleton.keys) != nil {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary source scalar", ErrInvalidLaneFactor)
		}
		position := -1
		for index, slot := range familyPlan.slots {
			equal, equalErr := d.CoordinateSlotEqual(slot, scalar.slot)
			if equalErr != nil {
				return CoordinateFamilySkeleton{}, nil, equalErr
			}
			if equal {
				position = index
				break
			}
		}
		if position < 0 || seen[position] {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary source scalar", ErrInvalidLaneFactor)
		}
		seen[position] = true
		support := coordinate.ops.scalarSupport(selected, scalar.slot.key)
		if !support.valid() || support == CoordinateScalarForbidden {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary source support", ErrInvalidLaneFactor)
		}
		projectedKey, keep, valid := coordinate.boundary.projectKey(&projectCtx, scalar.slot.key)
		if !valid || !keep {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: formal boundary source scalar is outside image", ErrInvalidLaneFactor)
		}
		projectedScalar, valid := coordinate.boundary.projectScalar(&projectCtx, projectedKey, scalar.payload)
		if !valid || projectedScalar == nil {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: formal boundary scalar projection failed in family %q", family.id)
		}
		targetKey, mapped := coordinate.ops.formalRekey.key(projectedKey, plan.projection.inverse)
		if !mapped || targetKey == nil || !coordinate.ops.keyValid(targetKey, targetSkeleton.keys) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: formal boundary key rekey failed in family %q", family.id)
		}
		targetScalar, imported := coordinate.ops.importScalar(projectedScalar)
		if !imported || targetScalar == nil || !coordinate.ops.scalarValid(targetKey, targetScalar) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: formal boundary scalar rekey failed in family %q", family.id)
		}
		out[scalarIndex] = CoordinateScalarFactor{slot: CoordinateSlot{family: family, keys: targetSkeleton.keys, key: targetKey}, payload: targetScalar}
	}
	return targetSkeleton, out, nil
}
