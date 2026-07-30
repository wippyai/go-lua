package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// CoordinateFamilyShape is a sealed dependent coordinate family: one
// structural skeleton together with the complete supported scalar inventory
// used by a transaction. Scalar values are deliberately not retained.
type CoordinateFamilyShape struct {
	domain   ProductDomain
	skeleton CoordinateFamilySkeleton
	slots    []CoordinateSlot
}

// SealCoordinateFamilyShape validates a family inventory against its
// skeleton and proves that every family-required scalar coordinate is
// represented. Forbidden candidates are structural absence and are removed.
func (d ProductDomain) SealCoordinateFamilyShape(skeleton CoordinateFamilySkeleton, candidates []CoordinateSlot) (CoordinateFamilyShape, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil {
		return CoordinateFamilyShape{}, err
	}
	admitted := make([]coordinateKeyPayload, 0, len(candidates))
	for index, slot := range candidates {
		if slot.family != skeleton.family || slot.keys != skeleton.keys ||
			d.validateCoordinateSlotFor(coordinate, slot, skeleton.keys) != nil {
			return CoordinateFamilyShape{}, fmt.Errorf("%w: coordinate family shape candidate %d", ErrInvalidLaneFactor, index)
		}
		admitted = append(admitted, slot.key)
	}
	sealed, post, ok := coordinate.ops.sealSkeletonInventory(skeleton.payload, admitted, skeleton.keys)
	if !ok || sealed == nil {
		return CoordinateFamilyShape{}, fmt.Errorf("%w: coordinate family shape normalization", ErrInvalidLaneFactor)
	}
	if len(post) != 0 {
		return CoordinateFamilyShape{}, fmt.Errorf("%w: coordinate family shape requires scalar completion", ErrIncompleteLaneFactors)
	}
	skeleton.payload = sealed
	supported := make([]CoordinateSlot, 0, len(candidates))
	for _, slot := range candidates {
		support := coordinate.ops.scalarSupport(skeleton.payload, slot.key)
		if !support.valid() {
			return CoordinateFamilyShape{}, fmt.Errorf("%w: coordinate family shape support", ErrInvalidLaneFactor)
		}
		if support != CoordinateScalarForbidden {
			supported = append(supported, slot)
		}
	}
	inventory, err := d.SealCoordinateFactorInventory(skeleton.keys, supported)
	if err != nil {
		return CoordinateFamilyShape{}, err
	}
	slots, err := inventory.FamilySlots(skeleton.family)
	if err != nil {
		return CoordinateFamilyShape{}, err
	}
	for _, key := range coordinate.ops.requiredScalarKeys(skeleton.payload) {
		if key == nil || !coordinate.ops.keyValid(key, skeleton.keys) ||
			coordinate.ops.scalarSupport(skeleton.payload, key) != CoordinateScalarRequired {
			return CoordinateFamilyShape{}, fmt.Errorf("%w: coordinate family required inventory law", ErrInvalidLaneFactor)
		}
		_, present, searchErr := coordinateShapeSlotIndex(d, slots, CoordinateSlot{family: skeleton.family, keys: skeleton.keys, key: key})
		if searchErr != nil {
			return CoordinateFamilyShape{}, searchErr
		}
		if !present {
			return CoordinateFamilyShape{}, fmt.Errorf("%w: coordinate family required scalar is absent", ErrIncompleteLaneFactors)
		}
	}
	return CoordinateFamilyShape{domain: d, skeleton: skeleton, slots: slots}, nil
}

func (s CoordinateFamilyShape) Skeleton() CoordinateFamilySkeleton { return s.skeleton }
func (s CoordinateFamilyShape) Slots() []CoordinateSlot {
	return append([]CoordinateSlot(nil), s.slots...)
}

func (s CoordinateFamilyShape) valid() bool {
	return s.domain.Valid() && s.skeleton.payload != nil && s.skeleton.family.seal == s.domain.seal

}

func (s CoordinateFamilyShape) validFor(d ProductDomain) bool {
	return s.valid() && s.domain.seal == d.seal
}

// CoordinateBoundaryWire is an opaque, lift-owned affected-fiber token.
// Accessors reveal dependency indexes only; the family algebra remains in
// state.
type CoordinateBoundaryWire struct {
	owner *coordinateBoundaryLiftSeal
	index int
}

type coordinateBoundaryLiftSeal struct{}

type coordinateBoundaryWirePlan struct {
	slot             CoordinateSlot
	destinationIndex int
	targetIndex      int
	sourceIndexes    []int
	rootIndexes      []int
}

type coordinateBoundaryOutputAction struct {
	destinationIndex int
	wireIndex        int
}

// CoordinateBoundaryFamilyLift is the single state-owned boundary
// transaction for one dependent coordinate family. Its wire set is sorted,
// complete for the final skeleton, and sparse in the affected cone.
type CoordinateBoundaryFamilyLift struct {
	seal        *coordinateBoundaryLiftSeal
	domain      ProductDomain
	route       coordinateBoundaryRoutePlan
	source      CoordinateFamilyShape
	destination CoordinateFamilyShape
	output      CoordinateFamilyShape
	wires       []coordinateBoundaryWirePlan
	actions     []coordinateBoundaryOutputAction
}

func (w CoordinateBoundaryWire) validFor(l CoordinateBoundaryFamilyLift) bool {
	return w.owner != nil && w.owner == l.seal && w.index >= 0 && w.index < len(l.wires)
}

func (w CoordinateBoundaryWire) Slot(l CoordinateBoundaryFamilyLift) (CoordinateSlot, bool) {
	if !w.validFor(l) {
		return CoordinateSlot{}, false
	}
	return l.wires[w.index].slot, true
}

func (w CoordinateBoundaryWire) DestinationIndex(l CoordinateBoundaryFamilyLift) (int, bool) {
	if !w.validFor(l) || l.wires[w.index].destinationIndex < 0 {
		return 0, false
	}
	return l.wires[w.index].destinationIndex, true
}

// SourceCount and SourceIndex expose the lift-owned source operand vector
// without copying it.  The wire seal is the bounds/ownership proof; callers
// cannot manufacture an index for another lift.
func (w CoordinateBoundaryWire) SourceCount(l CoordinateBoundaryFamilyLift) int {
	if !w.validFor(l) {
		return 0
	}
	return len(l.wires[w.index].sourceIndexes)
}

func (w CoordinateBoundaryWire) SourceIndex(l CoordinateBoundaryFamilyLift, index int) (int, bool) {
	if !w.validFor(l) || index < 0 || index >= len(l.wires[w.index].sourceIndexes) {
		return 0, false
	}
	return l.wires[w.index].sourceIndexes[index], true
}

// RootCount and RootIndex are the root-tuple counterpart of SourceCount and
// SourceIndex.  They keep the hot evaluator on plan-sealed operands rather
// than allocating defensive index copies for every affected fiber.
func (w CoordinateBoundaryWire) RootCount(l CoordinateBoundaryFamilyLift) int {
	if !w.validFor(l) {
		return 0
	}
	return len(l.wires[w.index].rootIndexes)
}

func (w CoordinateBoundaryWire) RootIndex(l CoordinateBoundaryFamilyLift, index int) (int, bool) {
	if !w.validFor(l) || index < 0 || index >= len(l.wires[w.index].rootIndexes) {
		return 0, false
	}
	return l.wires[w.index].rootIndexes[index], true
}

func (l CoordinateBoundaryFamilyLift) OutputShape() CoordinateFamilyShape { return l.output }
func (l CoordinateBoundaryFamilyLift) OutputCount() int                   { return len(l.actions) }
func (l CoordinateBoundaryFamilyLift) OutputSlot(index int) (CoordinateSlot, bool) {
	if index < 0 || index >= len(l.actions) || index >= len(l.output.slots) {
		return CoordinateSlot{}, false
	}
	return l.output.slots[index], true
}
func (l CoordinateBoundaryFamilyLift) OutputWire(index int) (CoordinateBoundaryWire, bool) {
	if index < 0 || index >= len(l.actions) || l.actions[index].wireIndex < 0 {
		return CoordinateBoundaryWire{}, false
	}
	return l.Wire(l.actions[index].wireIndex), true
}
func (l CoordinateBoundaryFamilyLift) OutputDestinationIndex(index int) (int, bool) {
	if index < 0 || index >= len(l.actions) || l.actions[index].destinationIndex < 0 {
		return 0, false
	}
	return l.actions[index].destinationIndex, true
}
func (l CoordinateBoundaryFamilyLift) FindOutput(slot CoordinateSlot) (int, bool, error) {
	if l.seal == nil || slot.family != l.output.skeleton.family || slot.keys != l.output.skeleton.keys {
		return 0, false, ErrInvalidLaneFactor
	}
	return coordinateShapeSlotIndex(l.domain, l.output.slots, slot)
}
func (l CoordinateBoundaryFamilyLift) SourceSlot(index int) (CoordinateSlot, bool) {
	if l.seal == nil || index < 0 || index >= len(l.source.slots) {
		return CoordinateSlot{}, false
	}
	return l.source.slots[index], true
}
func (l CoordinateBoundaryFamilyLift) WireCount() int { return len(l.wires) }
func (l CoordinateBoundaryFamilyLift) Wire(index int) CoordinateBoundaryWire {
	if index < 0 || index >= len(l.wires) {
		return CoordinateBoundaryWire{}
	}
	return CoordinateBoundaryWire{owner: l.seal, index: index}
}
func coordinateShapeSlotIndex(d ProductDomain, slots []CoordinateSlot, wanted CoordinateSlot) (int, bool, error) {
	coordinate, err := d.validateCoordinateFamily(wanted.family)
	if err != nil || wanted.keys == nil || wanted.family.seal != d.seal {
		return 0, false, ErrInvalidLaneFactor
	}
	index := sort.Search(len(slots), func(index int) bool {
		return !coordinate.ops.keyLess(slots[index].key, wanted.key, wanted.keys)
	})
	if index >= len(slots) || slots[index].family != wanted.family || slots[index].keys != wanted.keys {
		return index, false, nil
	}
	return index, coordinate.ops.keyEqual(slots[index].key, wanted.key), nil
}

// PrepareCoordinateBoundaryFamilyLift prepares the final family skeleton and
// its exact affected wires. Structural projection, inverse-fiber admission,
// skeleton restriction, and required-fiber proof all happen here once.
func (p BoundaryFactorTransportPlan) PrepareCoordinateBoundaryFamilyLift(source, destination CoordinateFamilyShape, establishesReachability bool) (CoordinateBoundaryFamilyLift, error) {
	if !source.validFor(p.sourceDomain) || !destination.validFor(p.domain) ||
		source.skeleton.family.id != destination.skeleton.family.id || source.skeleton.keys != p.projectCtx.keys || destination.skeleton.keys != p.keys {
		return CoordinateBoundaryFamilyLift{}, fmt.Errorf("%w: coordinate boundary family shape", ErrInvalidLaneFactor)
	}
	route, err := p.prepareCoordinateFamilyRoute(source.skeleton, source.slots)
	if err != nil {
		return CoordinateBoundaryFamilyLift{}, err
	}
	finalSkeleton, err := route.applySkeleton(destination.skeleton, establishesReachability)
	if err != nil {
		return CoordinateBoundaryFamilyLift{}, err
	}
	candidates := append([]CoordinateSlot(nil), destination.slots...)
	for target := 0; target < route.targetCount(); target++ {
		slot, ok := route.targetSlot(target)
		if !ok {
			return CoordinateBoundaryFamilyLift{}, ErrInvalidLaneFactor
		}
		candidates = append(candidates, slot)
	}
	output, err := p.domain.SealCoordinateFamilyShape(finalSkeleton, candidates)
	if err != nil {
		return CoordinateBoundaryFamilyLift{}, err
	}
	lift := CoordinateBoundaryFamilyLift{
		seal: &coordinateBoundaryLiftSeal{}, domain: p.domain, route: route,
		source: source, destination: destination, output: output,
	}
	for _, key := range route.coordinate.ops.requiredScalarKeys(route.fragmentSkeleton.payload) {
		targetIndex, hasTarget := route.findTarget(CoordinateSlot{family: route.coordinate.family, keys: p.keys, key: key})
		if !hasTarget || !route.targetHasFragment(targetIndex) && len(route.targets[targetIndex].roots) == 0 {
			return CoordinateBoundaryFamilyLift{}, fmt.Errorf("%w: required boundary fragment coordinate has no producer", ErrIncompleteLaneFactors)
		}
	}
	destinationIndex, targetIndex := 0, 0
	coordinate := route.coordinate
	for _, slot := range output.slots {
		for destinationIndex < len(destination.slots) && coordinate.ops.keyLess(destination.slots[destinationIndex].key, slot.key, p.keys) {
			destinationIndex++
		}
		hasDestination := destinationIndex < len(destination.slots) && coordinate.ops.keyEqual(destination.slots[destinationIndex].key, slot.key)
		for targetIndex < len(route.targets) && coordinate.ops.keyLess(route.targets[targetIndex].slot.key, slot.key, p.keys) {
			targetIndex++
		}
		hasTarget := targetIndex < len(route.targets) && coordinate.ops.keyEqual(route.targets[targetIndex].slot.key, slot.key)
		affected, affectedErr := route.destinationScalarAffected(slot)
		if affectedErr != nil {
			return CoordinateBoundaryFamilyLift{}, affectedErr
		}
		rootIndexes := make([]int, 0)
		if hasTarget {
			rootIndexes = append(rootIndexes, route.targets[targetIndex].roots...)
			if len(rootIndexes) > 1 {
				rootIndexes = rootIndexes[len(rootIndexes)-1:]
			}
		}
		if !hasTarget && !affected && len(rootIndexes) == 0 {
			if !hasDestination {
				return CoordinateBoundaryFamilyLift{}, fmt.Errorf("%w: required output coordinate has no producer", ErrIncompleteLaneFactors)
			}
			lift.actions = append(lift.actions, coordinateBoundaryOutputAction{destinationIndex: destinationIndex, wireIndex: -1})
			continue
		}
		wire := coordinateBoundaryWirePlan{slot: slot, destinationIndex: -1, targetIndex: -1, rootIndexes: rootIndexes}
		// A root publication is a complete last-write scalar override. Reading
		// destination or inverse-fiber operands would be both redundant and, for
		// a newly required root, structurally invalid.
		if hasDestination && len(rootIndexes) == 0 {
			wire.destinationIndex = destinationIndex
		}
		if hasTarget {
			wire.targetIndex = targetIndex
			if len(rootIndexes) == 0 {
				wire.sourceIndexes = route.targetSourceIndexes(targetIndex)
			}
		}
		action := coordinateBoundaryOutputAction{destinationIndex: -1, wireIndex: len(lift.wires)}
		if hasDestination {
			action.destinationIndex = destinationIndex
		}
		lift.actions = append(lift.actions, action)
		lift.wires = append(lift.wires, wire)
	}
	return lift, nil
}

// EvaluateWire evaluates exactly one affected fiber using the state-owned
// family algebra. Inputs must follow the lift's sealed dependency indexes.
func (l CoordinateBoundaryFamilyLift) EvaluateWire(w CoordinateBoundaryWire, destination *CoordinateScalarFactor, sources []CoordinateScalarFactor, roots []product.Value) (CoordinateScalarFactor, error) {
	if !w.validFor(l) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary wire", ErrInvalidLaneFactor)
	}
	plan := l.wires[w.index]
	if len(sources) != len(plan.sourceIndexes) || len(roots) != len(plan.rootIndexes) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary wire inputs", ErrInvalidLaneFactor)
	}
	if len(plan.rootIndexes) != 0 {
		if destination != nil || len(sources) != 0 || len(roots) != 1 || plan.targetIndex < 0 {
			return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary root wire inputs", ErrInvalidLaneFactor)
		}
		if !product.BelongsToRegistry(l.domain.reg, roots[0]) {
			return CoordinateScalarFactor{}, ErrInvalidLaneFactor
		}
		return l.route.applyRootScalar(plan.targetIndex, roots[0])
	}
	var current CoordinateScalarFactor
	if plan.destinationIndex >= 0 {
		if destination == nil {
			return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary wire destination", ErrInvalidLaneFactor)
		}
		equal, equalErr := l.domain.CoordinateSlotEqual(destination.slot, l.destination.slots[plan.destinationIndex])
		if equalErr != nil || !equal {
			return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary wire destination", ErrInvalidLaneFactor)
		}
		current = *destination
	} else {
		if destination != nil {
			return CoordinateScalarFactor{}, fmt.Errorf("%w: unexpected coordinate boundary destination", ErrInvalidLaneFactor)
		}
		coordinate, err := l.domain.validateCoordinateSkeleton(l.destination.skeleton)
		if err != nil {
			return CoordinateScalarFactor{}, err
		}
		payload, defaultErr := coordinate.ops.defaultScalar(l.destination.skeleton.payload, plan.slot.key)
		if defaultErr != nil {
			return CoordinateScalarFactor{}, defaultErr
		}
		current = CoordinateScalarFactor{slot: plan.slot, payload: payload}
	}
	var err error
	if plan.targetIndex >= 0 && l.route.targetHasFragment(plan.targetIndex) {
		current, err = l.route.applyTargetScalar(plan.targetIndex, current, sources)
	} else {
		current, err = l.route.applyDestinationScalar(current)
	}
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	return current, nil
}

func coordinateShapeCompatible(d ProductDomain, left CoordinateFamilyShape, skeleton CoordinateFamilySkeleton, scalars []CoordinateScalarFactor) (bool, error) {
	// A family's semantic skeleton quotient may be coarser than its retained
	// factorization. Scalar support and omitted defaults are substitutable only
	// under the registration-owned representation equality law.
	equal, err := d.CoordinateSkeletonRepresentationEqual(left.skeleton, skeleton)
	if err != nil || !equal {
		return false, err
	}
	for _, scalar := range scalars {
		_, found, findErr := coordinateShapeSlotIndex(left.domain, left.slots, scalar.slot)
		if findErr != nil || !found {
			return false, findErr
		}
	}
	for index, slot := range left.slots {
		support, supportErr := d.CoordinateScalarSupport(left.skeleton, slot)
		if supportErr != nil {
			return false, supportErr
		}
		if support != CoordinateScalarRequired {
			continue
		}
		_, present, scalarErr := coordinateScalarForShapeSlot(d, left, scalars, index)
		if scalarErr != nil || !present {
			return false, scalarErr
		}
	}
	return true, nil
}

func coordinateScalarForShapeSlot(d ProductDomain, shape CoordinateFamilyShape, scalars []CoordinateScalarFactor, index int) (CoordinateScalarFactor, bool, error) {
	if index < 0 || index >= len(shape.slots) {
		return CoordinateScalarFactor{}, false, ErrInvalidLaneFactor
	}
	slot := shape.slots[index]
	scalarIndex, found, searchErr := coordinateShapeScalarIndex(d, scalars, slot)
	if searchErr != nil {
		return CoordinateScalarFactor{}, false, searchErr
	}
	if found {
		return scalars[scalarIndex], true, nil
	}
	support, err := d.CoordinateScalarSupport(shape.skeleton, slot)
	if err != nil {
		return CoordinateScalarFactor{}, false, err
	}
	if support == CoordinateScalarRequired {
		return CoordinateScalarFactor{}, false, fmt.Errorf("%w: required coordinate scalar is absent", ErrIncompleteLaneFactors)
	}
	if support != CoordinateScalarOptional {
		return CoordinateScalarFactor{}, false, ErrInvalidLaneFactor
	}
	value, err := d.CoordinateDefault(shape.skeleton, slot)
	if err != nil {
		return CoordinateScalarFactor{}, false, err
	}
	return value, false, nil
}

func coordinateShapeScalarIndex(d ProductDomain, scalars []CoordinateScalarFactor, wanted CoordinateSlot) (int, bool, error) {
	coordinate, err := d.validateCoordinateFamily(wanted.family)
	if err != nil || wanted.keys == nil || wanted.family.seal != d.seal {
		return 0, false, ErrInvalidLaneFactor
	}
	index := sort.Search(len(scalars), func(index int) bool {
		return !coordinate.ops.keyLess(scalars[index].slot.key, wanted.key, wanted.keys)
	})
	if index >= len(scalars) || scalars[index].slot.family != wanted.family || scalars[index].slot.keys != wanted.keys {
		return index, false, nil
	}
	return index, coordinate.ops.keyEqual(scalars[index].slot.key, wanted.key), nil
}

// Seal applies every affected wire exactly once, carries only proven
// unaffected destination fibers, quotients Optional defaults, and composes the
// complete lane while preserving sibling coordinate families.
func (l CoordinateBoundaryFamilyLift) Seal(destination LaneFactor, evaluated []CoordinateScalarFactor) (LaneFactor, error) {
	if l.seal == nil || l.domain.seal == nil || destination.lane != l.destination.skeleton.family.lane {
		return LaneFactor{}, ErrInvalidLaneFactor
	}
	families, err := l.domain.CoordinateFamilies(destination.lane)
	if err != nil {
		return LaneFactor{}, err
	}
	skeletons := make([]CoordinateFamilySkeleton, len(families))
	factors := make([][]CoordinateScalarFactor, len(families))
	familyIndex := -1
	for index, family := range families {
		skeletons[index], factors[index], err = l.domain.DecomposeCoordinateFamily(destination, family, l.destination.skeleton.keys)
		if err != nil {
			return LaneFactor{}, err
		}
		if family == l.destination.skeleton.family {
			familyIndex = index
		}
	}
	if familyIndex < 0 {
		return LaneFactor{}, ErrInvalidLaneFactor
	}
	validShape, err := coordinateShapeCompatible(l.domain, l.destination, skeletons[familyIndex], factors[familyIndex])
	if err != nil || !validShape {
		return LaneFactor{}, fmt.Errorf("%w: coordinate boundary destination shape drift", ErrInvalidLaneFactor)
	}
	if len(evaluated) != len(l.wires) {
		return LaneFactor{}, fmt.Errorf("%w: incomplete coordinate boundary wires", ErrIncompleteLaneFactors)
	}
	result := make([]CoordinateScalarFactor, 0, len(l.output.slots))
	if len(l.actions) != len(l.output.slots) {
		return LaneFactor{}, ErrInvalidLaneFactor
	}
	for outputIndex, slot := range l.output.slots {
		action := l.actions[outputIndex]
		var value CoordinateScalarFactor
		if action.wireIndex >= 0 {
			value = evaluated[action.wireIndex]
			equal, equalErr := l.domain.CoordinateSlotEqual(value.slot, slot)
			if equalErr != nil || !equal {
				return LaneFactor{}, ErrInvalidLaneFactor
			}
		} else {
			if action.destinationIndex < 0 {
				return LaneFactor{}, ErrIncompleteLaneFactors
			}
			value, _, err = coordinateScalarForShapeSlot(l.domain, l.destination, factors[familyIndex], action.destinationIndex)
			if err != nil {
				return LaneFactor{}, err
			}
		}
		omitted, omitErr := l.domain.CoordinateScalarIsOmitted(l.output.skeleton, value)
		if omitErr != nil {
			return LaneFactor{}, omitErr
		}
		if !omitted {
			result = append(result, value)
		}
	}
	skeletons[familyIndex] = l.output.skeleton
	factors[familyIndex] = result
	return l.domain.ComposeCoordinateFamilies(destination.lane, l.destination.skeleton.keys, skeletons, factors)
}

// Apply materializes the same sparse lift for the concrete engine. It is a
// convenience only; all semantics flow through EvaluateWire and Seal.
func (l CoordinateBoundaryFamilyLift) Apply(destination, source LaneFactor, roots []product.Value) (LaneFactor, error) {
	if len(roots) != len(l.route.base.targets) {
		return LaneFactor{}, fmt.Errorf("%w: coordinate boundary root tuple", ErrInvalidLaneFactor)
	}
	sourceSkeleton, sourceScalars, err := l.source.domain.DecomposeCoordinateFamily(source, l.source.skeleton.family, l.source.skeleton.keys)
	if err != nil {
		return LaneFactor{}, err
	}
	valid, err := coordinateShapeCompatible(l.source.domain, l.source, sourceSkeleton, sourceScalars)
	if err != nil || !valid {
		return LaneFactor{}, ErrInvalidLaneFactor
	}
	_, destinationScalars, err := l.domain.DecomposeCoordinateFamily(destination, l.destination.skeleton.family, l.destination.skeleton.keys)
	if err != nil {
		return LaneFactor{}, err
	}
	evaluated := make([]CoordinateScalarFactor, len(l.wires))
	for index := range l.wires {
		wire := l.Wire(index)
		plan := l.wires[index]
		var destinationValue *CoordinateScalarFactor
		if plan.destinationIndex >= 0 {
			value, _, valueErr := coordinateScalarForShapeSlot(l.domain, l.destination, destinationScalars, plan.destinationIndex)
			if valueErr != nil {
				return LaneFactor{}, valueErr
			}
			destinationValue = &value
		}
		sourceValues := make([]CoordinateScalarFactor, len(plan.sourceIndexes))
		for input, sourceIndex := range plan.sourceIndexes {
			value, _, valueErr := coordinateScalarForShapeSlot(l.source.domain, l.source, sourceScalars, sourceIndex)
			if valueErr != nil {
				return LaneFactor{}, valueErr
			}
			sourceValues[input] = value
		}
		rootValues := make([]product.Value, len(plan.rootIndexes))
		for input, rootIndex := range plan.rootIndexes {
			if rootIndex < 0 || rootIndex >= len(roots) {
				return LaneFactor{}, ErrInvalidLaneFactor
			}
			rootValues[input] = roots[rootIndex]
		}
		evaluated[index], err = l.EvaluateWire(wire, destinationValue, sourceValues, rootValues)
		if err != nil {
			return LaneFactor{}, err
		}
	}
	return l.Seal(destination, evaluated)
}
