package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// PrepareHeapBoundaryPatch projects and rebases only heap topology. Source
// scalar values remain absent: returned selections reference the original
// source slots, and HeapBoundaryPlan.MapFragmentValue owns their later unary
// projection/rebase. Quotient collisions remain explicit source lists.
func (d ProductDomain) PrepareHeapBoundaryPatch(
	transport *BoundaryTransport,
	selection BoundaryFactorSelection,
	skeleton HeapTableIdentitySkeletonFactor,
	roots []HeapObjectRootSlot,
	members []HeapStaticMemberSlot,
) (HeapBoundaryPatch, error) {
	sourceKeys, closure := selection.keys, selection.closure
	if !d.Valid() || transport == nil || transport.authority == nil || transport.toKeys == nil || !transport.toKeys.Valid() ||
		!selection.valid() || sourceKeys == nil || !sourceKeys.Valid() || !boundaryAllocationAuthorityCovers(closure, transport.authority) {
		return HeapBoundaryPatch{}, fmt.Errorf("%w: factored heap boundary transport is unowned", ErrInvalidLaneFactor)
	}
	if _, err := d.validateHeapTableIdentitySkeleton(skeleton, sourceKeys); err != nil {
		return HeapBoundaryPatch{}, err
	}
	rootInventory, memberInventory, err := d.validateHeapBoundarySlotInventory(skeleton, roots, members, sourceKeys)
	if err != nil {
		return HeapBoundaryPatch{}, err
	}
	targetClosure, err := rebaseFactoredIdentityClosure(sourceKeys, transport.toKeys, closure, transport.authority)
	if err != nil {
		return HeapBoundaryPatch{}, err
	}
	lane, _ := d.ProductLane(LaneHeapTableIdentity)
	outSkeleton := HeapTableIdentitySkeletonFactor{seal: d.seal, lane: lane, keys: transport.toKeys, top: skeleton.top}
	patch := HeapBoundaryPatch{
		domain: d, keys: transport.toKeys, closure: targetClosure, skeleton: outSkeleton,
		roots: make(map[identity.Term]HeapObjectRootFactor), members: make(map[heapBoundaryMemberCoordinate]HeapStaticMemberFactor),
		fragmentRootSources:   make(map[identity.Term][]HeapObjectRootSlot),
		fragmentMemberSources: make(map[heapBoundaryMemberCoordinate][]HeapStaticMemberSlot),
	}
	rebaseContext := boundaryRebaseContext{reg: d.reg, fromKeys: sourceKeys, toKeys: transport.toKeys, allocations: transport.authority}
	patch.mapFragmentValue = func(value product.Value) (product.Value, error) {
		if !product.BelongsToRegistry(d.reg, value) {
			return product.Value{}, fmt.Errorf("%w: foreign heap boundary scalar", ErrInvalidLaneFactor)
		}
		value = product.ProjectBoundary(d.reg, value)
		next, ok := rebaseBoundaryValue(&rebaseContext, value)
		if !ok {
			return product.Value{}, fmt.Errorf("%w: heap boundary scalar identity has no substitution", ErrInvalidLaneFactor)
		}
		return next, nil
	}
	if skeleton.top {
		return patch, nil
	}
	outSkeleton.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton)
	for _, sourceID := range sortedHeapSkeletonIdentities(skeleton.objects) {
		if !closure.ContainsIdentityTerm(sourceID) {
			continue
		}
		sourceObject := skeleton.objects[sourceID]
		targetID, ok := rebaseBoundaryIdentity(transport.authority, sourceID)
		if !ok {
			return HeapBoundaryPatch{}, fmt.Errorf("%w: heap identity has no allocation substitution", ErrInvalidLaneFactor)
		}
		targetObject, keyMap, transformErr := d.projectRebaseHeapObjectSkeleton(&rebaseContext, sourceObject)
		if transformErr != nil {
			return HeapBoundaryPatch{}, transformErr
		}
		if prior, collision := outSkeleton.objects[targetID]; collision {
			targetObject = d.joinHeapObjectSkeleton(transport.toKeys, prior, targetObject, false)
		}
		outSkeleton.objects[targetID] = targetObject
		if !sourceObject.bottom {
			patch.fragmentRootSources[targetID] = append(patch.fragmentRootSources[targetID], rootInventory[sourceID])
			for _, sourceKey := range sourceObject.staticKeys {
				targetKey := keyMap[sourceKey]
				coordinate := heapBoundaryMemberCoordinate{id: targetID, key: targetKey}
				patch.fragmentMemberSources[coordinate] = append(patch.fragmentMemberSources[coordinate], memberInventory[heapBoundaryMemberCoordinate{id: sourceID, key: sourceKey}])
			}
		}
	}
	// Must-map collision joins retain only member keys present in every owner.
	// Delete source routes for keys absent from the exact joined skeleton.
	for coordinate := range patch.fragmentMemberSources {
		object, present := outSkeleton.objects[coordinate.id]
		if !present || object.bottom || !sortedHeapKeyContains(transport.toKeys, object.staticKeys, coordinate.key) {
			delete(patch.fragmentMemberSources, coordinate)
		}
	}
	for id := range patch.fragmentRootSources {
		sort.Slice(patch.fragmentRootSources[id], func(i, j int) bool {
			return identityTermLess(patch.fragmentRootSources[id][i].id, patch.fragmentRootSources[id][j].id)
		})
	}
	for coordinate := range patch.fragmentMemberSources {
		sort.Slice(patch.fragmentMemberSources[coordinate], func(i, j int) bool {
			left, right := patch.fragmentMemberSources[coordinate][i], patch.fragmentMemberSources[coordinate][j]
			if left.id != right.id {
				return identityTermLess(left.id, right.id)
			}
			return sourceKeys.Less(left.key, right.key)
		})
	}
	patch.skeleton = outSkeleton
	return patch, nil
}

// PreparePlacementBoundaryPatch is the scalar-free Placement counterpart.
// Rebased identity collisions are retained as source-slot lists; callers join
// their guarded scalar roots with the registered placement lattice.
func (d ProductDomain) PreparePlacementBoundaryPatch(
	transport *BoundaryTransport,
	selection BoundaryFactorSelection,
	skeleton PlacementSkeletonFactor,
	slots []PlacementSlot,
) (PlacementBoundaryPatch, error) {
	closure := selection.closure
	if !d.Valid() || transport == nil || transport.authority == nil || transport.toKeys == nil || !transport.toKeys.Valid() ||
		!selection.valid() || !boundaryAllocationAuthorityCovers(closure, transport.authority) {
		return PlacementBoundaryPatch{}, fmt.Errorf("%w: factored placement boundary transport is unowned", ErrInvalidLaneFactor)
	}
	if _, err := d.validatePlacementSkeleton(skeleton); err != nil {
		return PlacementBoundaryPatch{}, err
	}
	seen := make(map[identity.Term]struct{}, len(slots))
	for index, slot := range slots {
		if err := d.validatePlacementSlot(slot); err != nil {
			return PlacementBoundaryPatch{}, fmt.Errorf("%w: placement source slot %d", err, index)
		}
		if _, duplicate := seen[slot.id]; duplicate {
			return PlacementBoundaryPatch{}, fmt.Errorf("%w: duplicate placement source slot", ErrInvalidLaneFactor)
		}
		seen[slot.id] = struct{}{}
	}
	if skeleton.top && len(slots) != 0 {
		return PlacementBoundaryPatch{}, fmt.Errorf("%w: placement Top carries source slots", ErrInvalidLaneFactor)
	}
	targetClosure, err := rebaseFactoredIdentityClosure(selection.keys, transport.toKeys, closure, transport.authority)
	if err != nil {
		return PlacementBoundaryPatch{}, err
	}
	targetSkeleton, err := d.PlacementSkeletonBottom()
	if err != nil {
		return PlacementBoundaryPatch{}, err
	}
	targetSkeleton.top = skeleton.top
	patch := PlacementBoundaryPatch{
		domain: d, closure: targetClosure, skeleton: targetSkeleton,
		values: make(map[identity.Term]PlacementFactor), sources: make(map[identity.Term][]PlacementSlot),
	}
	if skeleton.top {
		return patch, nil
	}
	for _, slot := range slots {
		if !closure.ContainsIdentityTerm(slot.id) {
			continue
		}
		targetID, ok := rebaseBoundaryIdentity(transport.authority, slot.id)
		if !ok {
			return PlacementBoundaryPatch{}, fmt.Errorf("%w: placement identity has no allocation substitution", ErrInvalidLaneFactor)
		}
		patch.sources[targetID] = append(patch.sources[targetID], slot)
	}
	for id := range patch.sources {
		sort.Slice(patch.sources[id], func(i, j int) bool { return identityTermLess(patch.sources[id][i].id, patch.sources[id][j].id) })
	}
	return patch, nil
}

func (d ProductDomain) validateHeapBoundarySlotInventory(
	skeleton HeapTableIdentitySkeletonFactor,
	roots []HeapObjectRootSlot,
	members []HeapStaticMemberSlot,
	keys *keyspace.KeySpace,
) (map[identity.Term]HeapObjectRootSlot, map[heapBoundaryMemberCoordinate]HeapStaticMemberSlot, error) {
	rootMap := make(map[identity.Term]HeapObjectRootSlot, len(roots))
	for index, slot := range roots {
		if err := d.validateHeapObjectRootSlot(slot, keys); err != nil {
			return nil, nil, fmt.Errorf("%w: heap root slot %d", err, index)
		}
		if _, duplicate := rootMap[slot.id]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate heap root slot", ErrInvalidLaneFactor)
		}
		rootMap[slot.id] = slot
	}
	memberMap := make(map[heapBoundaryMemberCoordinate]HeapStaticMemberSlot, len(members))
	for index, slot := range members {
		if err := d.validateHeapStaticMemberSlot(slot, keys); err != nil {
			return nil, nil, fmt.Errorf("%w: heap member slot %d", err, index)
		}
		coordinate := heapBoundaryMemberCoordinate{id: slot.id, key: slot.key}
		if _, duplicate := memberMap[coordinate]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate heap member slot", ErrInvalidLaneFactor)
		}
		memberMap[coordinate] = slot
	}
	if skeleton.top {
		if len(rootMap)+len(memberMap) != 0 {
			return nil, nil, fmt.Errorf("%w: heap Top carries scalar slots", ErrInvalidLaneFactor)
		}
		return rootMap, memberMap, nil
	}
	expectedRoots, expectedMembers := 0, 0
	for id, object := range skeleton.objects {
		if object.bottom {
			continue
		}
		expectedRoots++
		if _, present := rootMap[id]; !present {
			return nil, nil, fmt.Errorf("%w: heap object %v has no root slot", ErrInvalidLaneFactor, id)
		}
		for _, memberKey := range object.staticKeys {
			expectedMembers++
			if _, present := memberMap[heapBoundaryMemberCoordinate{id: id, key: memberKey}]; !present {
				return nil, nil, fmt.Errorf("%w: heap object %v member has no slot", ErrInvalidLaneFactor, id)
			}
		}
	}
	if len(rootMap) != expectedRoots || len(memberMap) != expectedMembers {
		return nil, nil, fmt.Errorf("%w: heap scalar slot inventory has extras", ErrInvalidLaneFactor)
	}
	return rootMap, memberMap, nil
}

func (d ProductDomain) projectRebaseHeapObjectSkeleton(ctx *boundaryRebaseContext, source heapTableIdentityObjectSkeleton) (heapTableIdentityObjectSkeleton, map[keyspace.Key]keyspace.Key, error) {
	out := source
	keyMap := make(map[keyspace.Key]keyspace.Key, len(source.staticKeys))
	out.staticKeys = make([]keyspace.Key, len(source.staticKeys))
	for index, sourceKey := range source.staticKeys {
		targetKey, ok := ctx.toKeys.ImportKey(ctx.fromKeys, sourceKey)
		if !ok {
			return heapTableIdentityObjectSkeleton{}, nil, fmt.Errorf("%w: heap member key import", ErrInvalidLaneFactor)
		}
		keyMap[sourceKey], out.staticKeys[index] = targetKey, targetKey
	}
	sort.Slice(out.staticKeys, func(i, j int) bool { return ctx.toKeys.Less(out.staticKeys[i], out.staticKeys[j]) })
	if source.dynamicIndexFacts != nil {
		out.dynamicIndexFacts = make(map[dynamicindex.Key]dynamicindex.Fact, len(source.dynamicIndexFacts))
		for sourceKey, sourceFact := range source.dynamicIndexFacts {
			table, ok := ctx.toKeys.ImportKey(ctx.fromKeys, sourceKey.Table)
			if !ok {
				return heapTableIdentityObjectSkeleton{}, nil, fmt.Errorf("%w: heap dynamic key import", ErrInvalidLaneFactor)
			}
			keyValue, err := mapFactoredBoundaryProduct(ctx, sourceFact.KeyValue)
			if err != nil {
				return heapTableIdentityObjectSkeleton{}, nil, err
			}
			value, err := mapFactoredBoundaryProduct(ctx, sourceFact.Value)
			if err != nil {
				return heapTableIdentityObjectSkeleton{}, nil, err
			}
			sourceFact.KeyValue, sourceFact.Value = keyValue, value
			targetKey := dynamicindex.Key{Table: table, Site: sourceKey.Site}
			if prior, collision := out.dynamicIndexFacts[targetKey]; collision {
				sourceFact = dynamicindex.Domain(d.reg).Join(prior, sourceFact)
			}
			out.dynamicIndexFacts[targetKey] = sourceFact
		}
	}
	return out, keyMap, nil
}

func mapFactoredBoundaryProduct(ctx *boundaryRebaseContext, value product.Value) (product.Value, error) {
	value = product.ProjectBoundary(ctx.reg, value)
	next, ok := rebaseBoundaryValue(ctx, value)
	if !ok {
		return product.Value{}, fmt.Errorf("%w: boundary product identity has no substitution", ErrInvalidLaneFactor)
	}
	return next, nil
}

func rebaseFactoredIdentityClosure(from, to *keyspace.KeySpace, source BoundaryClosure, allocations *BoundaryAllocationAuthority) (BoundaryClosure, error) {
	out := emptyBoundaryClosure()
	out.allIdentities = source.allIdentities
	for sourceID := range source.identities {
		targetID, ok := rebaseBoundaryIdentity(allocations, sourceID)
		if !ok {
			return BoundaryClosure{}, fmt.Errorf("%w: closure identity has no substitution", ErrInvalidLaneFactor)
		}
		out.identities[targetID] = struct{}{}
	}
	for suffix := range source.heapSuffixes {
		targetID, ok := rebaseBoundaryIdentity(allocations, suffix.owner)
		if !ok {
			return BoundaryClosure{}, fmt.Errorf("%w: closure suffix identity has no substitution", ErrInvalidLaneFactor)
		}
		if from == nil {
			continue
		}
		targetKey, ok := to.ImportKey(from, suffix.suffix)
		if !ok {
			return BoundaryClosure{}, fmt.Errorf("%w: closure suffix key import", ErrInvalidLaneFactor)
		}
		out.heapSuffixes[boundaryHeapSuffix{owner: targetID, suffix: targetKey}] = struct{}{}
	}
	return out, nil
}
