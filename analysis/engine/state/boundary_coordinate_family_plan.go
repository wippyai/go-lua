package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// coordinateBoundaryRoutePlan is the registered, factorwise boundary routing
// of one coordinate family. It contains structural keys and source indexes,
// never scalar payloads. A consumer aligns only the source scalars that
// collide at one destination coordinate.
type coordinateBoundaryRoutePlan struct {
	base             BoundaryFactorTransportPlan
	coordinate       *coordinateFamilyRuntime
	sourceCoordinate *coordinateFamilyRuntime
	fragmentSkeleton CoordinateFamilySkeleton
	sourceSlots      []CoordinateSlot
	projectedKeys    []coordinateKeyPayload
	targets          []coordinateBoundaryTarget
}

type coordinateBoundaryTarget struct {
	slot    CoordinateSlot
	sources []int
	post    []coordinateScalarPayload
	roots   []int
}

type coordinateBoundaryContribution struct {
	source int
	fiber  coordinateFiberPayload
}

// CoordinateSourceFiberIndexes returns the exact source-coordinate cone
// admitted by this already-sealed boundary plan.  The result is expressed in
// the caller's slot inventory, in ascending inventory order.  Projection is a
// structural family law: scalar payloads and sibling families cannot affect
// this answer.
//
// Guarded execution uses this proof before aligning decision roots, so an
// unrelated coordinate never becomes a physical operand of the boundary
// transaction merely because it shares a registered lane with an affected
// coordinate.
func (p BoundaryFactorTransportPlan) CoordinateSourceFiberIndexes(family CoordinateFamily, slots []CoordinateSlot) ([]int, error) {
	coordinate, err := p.sourceDomain.validateCoordinateFamily(family)
	if err != nil || family.lane != coordinate.family.lane {
		return nil, fmt.Errorf("%w: invalid coordinate source family", ErrInvalidLaneFactor)
	}
	out := make([]int, 0, len(slots))
	for index, slot := range slots {
		if err := p.sourceDomain.validateCoordinateSlotFor(coordinate, slot, p.projectCtx.keys); err != nil {
			return nil, fmt.Errorf("%w: coordinate source fiber %d", ErrInvalidLaneFactor, index)
		}
		_, keep, valid := coordinate.boundary.projectKey(&p.projectCtx, slot.key)
		if !valid {
			return nil, fmt.Errorf("state: coordinate boundary key projection failed in family %q", coordinate.family.id)
		}
		if keep {
			out = append(out, index)
		}
	}
	return out, nil
}

// prepareCoordinateFamilyRoute computes Project→Rebase key routing once. Its
// target admission is the coordinate family's registered image law: pointwise
// may maps need one present preimage, while must-bearing families require their
// complete inverse fiber.
func (p BoundaryFactorTransportPlan) prepareCoordinateFamilyRoute(source CoordinateFamilySkeleton, sourceSlots []CoordinateSlot) (coordinateBoundaryRoutePlan, error) {
	sourceCoordinate, sourceErr := p.sourceDomain.validateCoordinateFamily(source.family)
	coordinate, err := p.domain.coordinateRuntimeForImport(source.family)
	if err != nil || sourceErr != nil || source.payload == nil || source.keys != p.projectCtx.keys {
		return coordinateBoundaryRoutePlan{}, fmt.Errorf("%w: invalid source coordinate family", ErrInvalidLaneFactor)
	}
	projectedSkeleton, ok := sourceCoordinate.boundary.projectSkeleton(&p.projectCtx, source.payload)
	if !ok || projectedSkeleton == nil {
		return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary skeleton projection failed in family %q", coordinate.family.id)
	}
	rebasedSkeleton, ok := coordinate.boundary.rebaseSkeleton(&p.rebaseCtx, projectedSkeleton)
	if !ok || rebasedSkeleton == nil {
		return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary skeleton rebase failed in family %q", coordinate.family.id)
	}
	post := coordinate.boundary.postEntries(p.aliases)
	out := coordinateBoundaryRoutePlan{
		base: p, coordinate: coordinate, sourceCoordinate: sourceCoordinate,
		fragmentSkeleton: CoordinateFamilySkeleton{family: coordinate.family, keys: p.keys, payload: rebasedSkeleton},
		sourceSlots:      append([]CoordinateSlot(nil), sourceSlots...),
		projectedKeys:    make([]coordinateKeyPayload, len(sourceSlots)),
	}
	contributions := make([][]coordinateBoundaryContribution, 0)
	targetBuckets := make(map[uint64][]int)
	findTarget := func(key coordinateKeyPayload) int {
		hash := coordinate.ops.keyHash(key, p.keys)
		for _, index := range targetBuckets[hash] {
			if coordinate.ops.keyEqual(out.targets[index].slot.key, key) {
				return index
			}
		}
		out.targets = append(out.targets, coordinateBoundaryTarget{slot: CoordinateSlot{family: coordinate.family, keys: p.keys, key: key}})
		contributions = append(contributions, nil)
		index := len(out.targets) - 1
		targetBuckets[hash] = append(targetBuckets[hash], index)
		return index
	}
	for sourceIndex, slot := range sourceSlots {
		if slot.family != source.family || slot.keys != source.keys || slot.key == nil || !sourceCoordinate.ops.keyValid(slot.key, source.keys) {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("%w: coordinate source slot %d differs from family", ErrInvalidLaneFactor, sourceIndex)
		}
		projected, keep, valid := sourceCoordinate.boundary.projectKey(&p.projectCtx, slot.key)
		if !valid {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary key projection failed in family %q", coordinate.family.id)
		}
		if !keep {
			continue
		}
		out.projectedKeys[sourceIndex] = projected
		fiber := sourceCoordinate.boundary.sourceFiber(projected)
		if fiber == nil {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary source fiber is empty in family %q", coordinate.family.id)
		}
		keys, mapped := coordinate.boundary.rebaseKeys(&p.rebaseCtx, projected)
		if !mapped {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary key rebase failed in family %q", coordinate.family.id)
		}
		for _, key := range keys {
			if key == nil || !coordinate.ops.keyValid(key, p.keys) {
				return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary emitted invalid key in family %q", coordinate.family.id)
			}
			target := findTarget(key)
			contributions[target] = append(contributions[target], coordinateBoundaryContribution{source: sourceIndex, fiber: fiber})
		}
	}
	for _, entry := range post {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, p.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary emitted invalid post entry in family %q", coordinate.family.id)
		}
		support := coordinate.ops.scalarSupport(rebasedSkeleton, entry.key)
		if !support.valid() {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary emitted unsupported post entry in family %q", coordinate.family.id)
		}
		if support == CoordinateScalarForbidden {
			continue
		}
		target := findTarget(entry.key)
		out.targets[target].post = append(out.targets[target].post, entry.scalar)
	}
	applyCtx := boundaryApplyContext{reg: p.domain.reg, keys: p.keys, closure: p.closure}
	for rootIndex, root := range p.targets {
		key, claimed, valid := coordinate.boundary.rootSlot(&applyCtx, root)
		if !valid {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary root claim failed in family %q", coordinate.family.id)
		}
		if claimed {
			if key == nil || !coordinate.ops.keyValid(key, p.keys) {
				return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary root claim emitted invalid key in family %q", coordinate.family.id)
			}
			target := findTarget(key)
			out.targets[target].roots = append(out.targets[target].roots, rootIndex)
		}
	}
	filtered := make([]coordinateBoundaryTarget, 0, len(out.targets))
	for index, target := range out.targets {
		if len(target.roots) != 0 || len(target.post) != 0 {
			target.sources = uniqueCoordinateSourceIndexes(contributions[index])
			filtered = append(filtered, target)
			continue
		}
		if len(contributions[index]) == 0 {
			continue
		}
		required, valid := coordinate.boundaryTargetRequiredFibers(
			&p.rebaseCtx, target.slot.key, contributions[index][0].fiber,
		)
		if !valid || len(required) == 0 {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary target admission failed in family %q", coordinate.family.id)
		}
		complete := true
		for _, fiber := range required {
			found := false
			for _, contribution := range contributions[index] {
				if contribution.fiber == fiber {
					found = true
					break
				}
			}
			if !found {
				complete = false
				break
			}
		}
		if complete {
			target.sources = uniqueCoordinateSourceIndexes(contributions[index])
			filtered = append(filtered, target)
		}
	}
	out.targets = filtered
	targetBuckets = make(map[uint64][]int, len(out.targets))
	for index := range out.targets {
		hash := coordinate.ops.keyHash(out.targets[index].slot.key, p.keys)
		targetBuckets[hash] = append(targetBuckets[hash], index)
	}
	admitted := make([]coordinateKeyPayload, len(out.targets))
	for index := range out.targets {
		admitted[index] = out.targets[index].slot.key
	}
	restricted, sealPost, restrictedOK := coordinate.ops.sealSkeletonInventory(rebasedSkeleton, admitted, p.keys)
	if !restrictedOK || restricted == nil {
		return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary fragment restriction failed in family %q", coordinate.family.id)
	}
	for _, entry := range sealPost {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, p.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) {
			return coordinateBoundaryRoutePlan{}, fmt.Errorf("state: coordinate boundary fragment seal emitted invalid entry in family %q", coordinate.family.id)
		}
		target := findTarget(entry.key)
		out.targets[target].post = append(out.targets[target].post, entry.scalar)
	}
	// The family seal can remove unsupported structure or emit an explicit
	// conservative witness. Coordinates forbidden by the sealed skeleton
	// disappear; omission is never confused with an invented scalar default.
	originalTargets := out.targets
	filtered = make([]coordinateBoundaryTarget, 0, len(originalTargets))
	for _, target := range originalTargets {
		if support := coordinate.ops.scalarSupport(restricted, target.slot.key); support.valid() && (support != CoordinateScalarForbidden || len(target.roots) != 0) {
			filtered = append(filtered, target)
		}
	}
	out.targets = filtered
	out.fragmentSkeleton.payload = restricted
	sort.Slice(out.targets, func(i, j int) bool {
		return coordinate.ops.keyLess(out.targets[i].slot.key, out.targets[j].slot.key, p.keys)
	})
	return out, nil
}

func uniqueCoordinateSourceIndexes(input []coordinateBoundaryContribution) []int {
	out := make([]int, 0, len(input))
	seen := make(map[int]struct{}, len(input))
	for _, value := range input {
		if _, ok := seen[value.source]; !ok {
			seen[value.source] = struct{}{}
			out = append(out, value.source)
		}
	}
	sort.Ints(out)
	return out
}

func (p coordinateBoundaryRoutePlan) targetCount() int { return len(p.targets) }

func (p coordinateBoundaryRoutePlan) targetSlot(index int) (CoordinateSlot, bool) {
	if index < 0 || index >= len(p.targets) {
		return CoordinateSlot{}, false
	}
	return p.targets[index].slot, true
}

func (p coordinateBoundaryRoutePlan) targetSourceIndexes(index int) []int {
	if index < 0 || index >= len(p.targets) {
		return nil
	}
	return append([]int(nil), p.targets[index].sources...)
}

func (p coordinateBoundaryRoutePlan) targetHasFragment(index int) bool {
	return index >= 0 && index < len(p.targets) && (len(p.targets[index].sources) != 0 || len(p.targets[index].post) != 0)
}

func (p coordinateBoundaryRoutePlan) findTarget(slot CoordinateSlot) (int, bool) {
	if slot.keys != p.base.keys || slot.family.id != p.coordinate.family.id || slot.key == nil {
		return 0, false
	}
	index := sort.Search(len(p.targets), func(index int) bool {
		return !p.coordinate.ops.keyLess(p.targets[index].slot.key, slot.key, p.base.keys)
	})
	return index, index < len(p.targets) && p.coordinate.ops.keyEqual(slot.key, p.targets[index].slot.key)
}

// destinationScalarAffected reports the exact closure-replacement ownership
// of an existing destination coordinate. False is a proof that applying this
// plan would return the destination scalar unchanged; callers may therefore
// carry its interned decision root without evaluating scalar algebra.
func (p coordinateBoundaryRoutePlan) destinationScalarAffected(slot CoordinateSlot) (bool, error) {
	if slot.keys != p.base.keys || slot.family.id != p.coordinate.family.id || slot.key == nil ||
		!p.coordinate.ops.keyValid(slot.key, p.base.keys) {
		return false, fmt.Errorf("%w: invalid coordinate boundary destination", ErrInvalidLaneFactor)
	}
	selector, err := p.coordinate.boundary.sealAffectedSelector(p.base.keys, slot.key)
	if err != nil {
		return false, err
	}
	return selector.affected(p.base.closure), nil
}

// applyDestinationScalar performs the closure delete/preserve law when no
// transported fragment owns this coordinate.
func (p coordinateBoundaryRoutePlan) applyDestinationScalar(destination CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	if destination.slot.keys != p.base.keys || !p.coordinate.ops.keyValid(destination.slot.key, p.base.keys) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: invalid coordinate boundary destination", ErrInvalidLaneFactor)
	}
	fragment, err := p.coordinate.ops.defaultScalar(p.fragmentSkeleton.payload, destination.slot.key)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	affected, err := p.destinationScalarAffected(destination.slot)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	payload, ok := p.coordinate.boundary.applyScalar(destination.slot.key, destination.payload, fragment, affected)
	if !ok {
		return CoordinateScalarFactor{}, fmt.Errorf("state: coordinate boundary destination apply failed in family %q", p.coordinate.family.id)
	}
	return CoordinateScalarFactor{slot: destination.slot, payload: payload}, nil
}

// applySkeleton applies only the family quotient and root reachability. No
// coordinate scalar is read by this transaction.
func (p coordinateBoundaryRoutePlan) applySkeleton(destination CoordinateFamilySkeleton, establishesReachability bool) (CoordinateFamilySkeleton, error) {
	if err := p.base.domain.validateCoordinateSkeletonFor(p.coordinate, destination, p.base.keys); err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	ctx := boundaryApplyContext{reg: p.base.domain.reg, keys: p.base.keys, closure: p.base.closure}
	payload, ok := p.coordinate.boundary.applySkeleton(&ctx, destination.payload, p.fragmentSkeleton.payload)
	if !ok {
		return CoordinateFamilySkeleton{}, fmt.Errorf("state: coordinate boundary skeleton apply failed in family %q", p.coordinate.family.id)
	}
	payload, ok = p.coordinate.boundary.applyRootSkeleton(&ctx, payload, establishesReachability)
	if !ok {
		return CoordinateFamilySkeleton{}, fmt.Errorf("state: coordinate boundary root skeleton apply failed in family %q", p.coordinate.family.id)
	}
	return CoordinateFamilySkeleton{family: p.coordinate.family, keys: p.base.keys, payload: payload}, nil
}

// applyTargetScalar maps and joins only the source scalars in this target's
// inverse fiber, then applies that fragment to the destination coordinate.
// sources must follow targetSourceIndexes(index) exactly.
func (p coordinateBoundaryRoutePlan) applyTargetScalar(index int, destination CoordinateScalarFactor, sources []CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	if index < 0 || index >= len(p.targets) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: invalid coordinate boundary target", ErrInvalidLaneFactor)
	}
	target := p.targets[index]
	if len(sources) != len(target.sources) || destination.slot.keys != p.base.keys || !p.coordinate.ops.keyEqual(destination.slot.key, target.slot.key) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary target inventory drift", ErrInvalidLaneFactor)
	}
	var fragment coordinateScalarPayload
	join := func(value coordinateScalarPayload) {
		if fragment == nil {
			fragment = value
		} else {
			fragment = p.coordinate.ops.scalarJoin(fragment, value)
		}
	}
	for input, sourceIndex := range target.sources {
		if sourceIndex < 0 || sourceIndex >= len(p.sourceSlots) || p.projectedKeys[sourceIndex] == nil {
			return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary source index drift", ErrInvalidLaneFactor)
		}
		source := sources[input]
		sourceSlot := p.sourceSlots[sourceIndex]
		if source.slot.family != sourceSlot.family || source.slot.keys != sourceSlot.keys || !p.sourceCoordinate.ops.keyEqual(source.slot.key, sourceSlot.key) || !p.sourceCoordinate.ops.scalarValid(source.slot.key, source.payload) {
			return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate boundary source scalar drift", ErrInvalidLaneFactor)
		}
		projected, ok := p.sourceCoordinate.boundary.projectScalar(&p.base.projectCtx, p.projectedKeys[sourceIndex], source.payload)
		if !ok {
			return CoordinateScalarFactor{}, fmt.Errorf("state: coordinate boundary scalar projection failed in family %q", p.coordinate.family.id)
		}
		rebased, ok := p.coordinate.boundary.rebaseScalar(&p.base.rebaseCtx, p.projectedKeys[sourceIndex], projected)
		if !ok {
			return CoordinateScalarFactor{}, fmt.Errorf("state: coordinate boundary scalar rebase failed in family %q", p.coordinate.family.id)
		}
		join(rebased)
	}
	for _, value := range target.post {
		join(value)
	}
	if fragment == nil || !p.coordinate.ops.scalarValid(target.slot.key, fragment) {
		return CoordinateScalarFactor{}, fmt.Errorf("state: coordinate boundary target has no valid fragment in family %q", p.coordinate.family.id)
	}
	affected, err := p.destinationScalarAffected(target.slot)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	payload, ok := p.coordinate.boundary.applyScalar(target.slot.key, destination.payload, fragment, affected)
	if !ok {
		return CoordinateScalarFactor{}, fmt.Errorf("state: coordinate boundary scalar apply failed in family %q", p.coordinate.family.id)
	}
	return CoordinateScalarFactor{slot: target.slot, payload: payload}, nil
}

func (p coordinateBoundaryRoutePlan) rootTarget(root int) (int, bool) {
	for target := range p.targets {
		for _, candidate := range p.targets[target].roots {
			if candidate == root {
				return target, true
			}
		}
	}
	return 0, false
}

// applyRootScalar wraps one already-rebased destination root as the exact
// coordinate written after ordinary family application.
func (p coordinateBoundaryRoutePlan) applyRootScalar(target int, value product.Value) (CoordinateScalarFactor, error) {
	if target < 0 || target >= len(p.targets) || !product.BelongsToRegistry(p.base.domain.reg, value) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: invalid coordinate boundary root scalar", ErrInvalidLaneFactor)
	}
	payload, ok := p.coordinate.boundary.rootScalar(
		&boundaryApplyContext{reg: p.base.domain.reg, keys: p.base.keys, closure: p.base.closure},
		p.targets[target].slot.key, value,
	)
	if !ok || !p.coordinate.ops.scalarValid(p.targets[target].slot.key, payload) {
		return CoordinateScalarFactor{}, fmt.Errorf("state: coordinate boundary root scalar failed in family %q", p.coordinate.family.id)
	}
	return CoordinateScalarFactor{slot: p.targets[target].slot, payload: payload}, nil
}
