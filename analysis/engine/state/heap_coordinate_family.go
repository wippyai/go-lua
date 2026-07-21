package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

const heapCoordinateFamilyID CoordinateFamilyID = "coupled-heap-object"

type heapCoordinateKind uint8

const (
	heapCoordinateRoot heapCoordinateKind = iota + 1
	heapCoordinateMember
)

type heapCoordinateSkeleton struct {
	keys    *keyspace.KeySpace
	top     bool
	objects map[identity.Term]heapTableIdentityObjectSkeleton
}

type heapCoordinateKey struct {
	kind heapCoordinateKind
	id   identity.Term
	key  keyspace.Key
}

type heapCoordinateScalar struct{ value product.Value }

type heapCoordinateOverlayPlan struct {
	selectedCount int
	byObject      map[identity.Term]*heapSelectedSkeletonObject
}

type heapCoordinateFiber struct {
	kind heapCoordinateKind
	id   identity.Term
	key  keyspace.Key
}

// HeapObjectRootSlotsFromCoordinateInventory projects the exact registered
// heap-root coordinates already admitted by a frozen scalar inventory. It
// performs no State scan and cannot discover topology at execution time.
func (d ProductDomain) HeapObjectRootSlotsFromCoordinateInventory(inventory CoordinateFactorInventory) ([]HeapObjectRootSlot, error) {
	if !inventory.ValidFor(d, inventory.KeySpace()) {
		return nil, fmt.Errorf("%w: heap-root coordinate inventory", ErrInvalidLaneFactor)
	}
	var out []HeapObjectRootSlot
	for _, bucket := range inventory.set.families {
		if bucket.family.id != heapCoordinateFamilyID {
			continue
		}
		for _, slot := range bucket.slots {
			key := heapCoordinateKeyValue(slot.key)
			if key.kind != heapCoordinateRoot {
				continue
			}
			out = append(out, HeapObjectRootSlot{seal: d.seal, lane: bucket.family.lane, keys: inventory.keys, id: key.id})
		}
	}
	return out, nil
}

func heapCoordinateSkeletonFromLegacy(source HeapTableIdentitySkeletonFactor) heapCoordinateSkeleton {
	return heapCoordinateSkeleton{keys: source.keys, top: source.top, objects: source.objects}
}

func legacyHeapCoordinateSkeleton(owner HeapTableIdentitySkeletonFactor, source heapCoordinateSkeleton) HeapTableIdentitySkeletonFactor {
	return HeapTableIdentitySkeletonFactor{
		seal: owner.seal, lane: owner.lane, keys: owner.keys,
		top: source.top, objects: source.objects,
	}
}

var heapCoordinateFamilySpec = coordinateFamilySpec{
	identityImage: IdentityImagePointwiseMap,
	id:            heapCoordinateFamilyID,
	build:         buildHeapCoordinateFamily,
	dynamicRead:   dynamicReadHeapCoordinates(),
	boundary: coordinateFamilyBoundaryOps{
		admission:       coordinateBoundaryAdmissionAnyPresent,
		rootUse:         boundaryRootUseNone(),
		reachabilityKey: func(*boundaryReachabilityProgramBuilder, coordinateKeyPayload) {},
		projectSkeleton: func(ctx *boundaryProjectContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			skeleton := heapCoordinateSkeletonValue(source)
			if skeleton.top {
				return source, true
			}
			out := heapCoordinateSkeleton{keys: skeleton.keys}
			for id, object := range skeleton.objects {
				if !ctx.closure.ContainsIdentityTerm(id) {
					continue
				}
				object = mapHeapCoordinateObjectValues(object, func(value product.Value) (product.Value, bool) {
					return product.ProjectBoundary(ctx.reg, value), true
				})
				if out.objects == nil {
					out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton)
				}
				out.objects[id] = object
			}
			return wrapHeapCoordinateSkeleton(out), true
		},
		projectKey: func(ctx *boundaryProjectContext, source coordinateKeyPayload) (coordinateKeyPayload, bool, bool) {
			key := heapCoordinateKeyValue(source)
			return source, ctx.closure.ContainsIdentityTerm(key.id), true
		},
		projectScalar: func(ctx *boundaryProjectContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return wrapHeapCoordinateScalar(product.ProjectBoundary(ctx.reg, heapCoordinateScalarValue(source).value)), true
		},
		rebaseSkeleton: func(ctx *boundaryRebaseContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			skeleton := heapCoordinateSkeletonValue(source)
			if skeleton.top {
				skeleton.keys = ctx.toKeys
				return wrapHeapCoordinateSkeleton(skeleton), true
			}
			out := heapCoordinateSkeleton{keys: ctx.toKeys}
			for id, object := range skeleton.objects {
				nextID, ok := rebaseBoundaryIdentity(ctx.allocations, id)
				if !ok {
					return nil, false
				}
				object, ok = importHeapCoordinateObjectKeys(ctx.reg, object, ctx.fromKeys, ctx.toKeys)
				if !ok {
					return nil, false
				}
				valid := true
				object = mapHeapCoordinateObjectValues(object, func(value product.Value) (product.Value, bool) {
					next, mapped := rebaseBoundaryProduct(ctx, value)
					valid = valid && mapped
					return next, mapped
				})
				if !valid {
					return nil, false
				}
				if existing, collision := out.objects[nextID]; collision {
					object = joinHeapCoordinateObject(ctx.reg, ctx.toKeys, existing, object, false)
				}
				if out.objects == nil {
					out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton)
				}
				out.objects[nextID] = object
			}
			return wrapHeapCoordinateSkeleton(out), true
		},
		rebaseKeys: func(ctx *boundaryRebaseContext, source coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			key := heapCoordinateKeyValue(source)
			id := key.id
			_, alreadyImaged := id.Formal()
			var ok bool
			if !ctx.identityImaged || !alreadyImaged {
				id, ok = rebaseBoundaryIdentity(ctx.allocations, id)
			} else {
				ok = true
			}
			if !ok {
				return nil, false
			}
			key.id = id
			if key.kind == heapCoordinateMember {
				key.key, ok = ctx.toKeys.ImportKey(ctx.fromKeys, key.key)
				if !ok {
					return nil, false
				}
			}
			return []coordinateKeyPayload{wrapHeapCoordinateKey(key)}, true
		},
		rebaseScalar: func(ctx *boundaryRebaseContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			value, ok := rebaseBoundaryProduct(ctx, heapCoordinateScalarValue(source).value)
			return wrapHeapCoordinateScalar(value), ok
		},
		sourceFiber: func(source coordinateKeyPayload) coordinateFiberPayload {
			key := heapCoordinateKeyValue(source)
			return typedCoordinateFiberPayload[heapCoordinateFiber]{value: heapCoordinateFiber{kind: key.kind, id: key.id, key: key.key}}
		},
		inverseFibers: func(ctx *boundaryRebaseContext, destination coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
			key := heapCoordinateKeyValue(destination)
			ids, ok := ctx.quotient.identityPreimages(key.id)
			if !ok {
				return nil, false
			}
			if key.kind == heapCoordinateMember {
				key.key, ok = ctx.fromKeys.ImportKey(ctx.toKeys, key.key)
				if !ok {
					return nil, false
				}
			}
			out := make([]coordinateFiberPayload, len(ids))
			for index, id := range ids {
				out[index] = typedCoordinateFiberPayload[heapCoordinateFiber]{value: heapCoordinateFiber{kind: key.kind, id: id, key: key.key}}
			}
			return out, true
		},
		postEntries: noCoordinatePostEntries,
		applySkeleton: func(ctx *boundaryApplyContext, destination, fragment coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			left, right := heapCoordinateSkeletonValue(destination), heapCoordinateSkeletonValue(fragment)
			if left.top || right.top {
				return wrapHeapCoordinateSkeleton(heapCoordinateSkeleton{keys: left.keys, top: true}), true
			}
			out := cloneHeapCoordinateSkeleton(left)
			for id := range out.objects {
				if ctx.closure.ContainsIdentityTerm(id) {
					delete(out.objects, id)
				}
			}
			if len(right.objects) != 0 && out.objects == nil {
				out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(right.objects))
			}
			for id, object := range right.objects {
				out.objects[id] = cloneHeapCoordinateObject(object)
			}
			if len(out.objects) == 0 {
				out.objects = nil
			}
			return wrapHeapCoordinateSkeleton(out), true
		},
		applyScalar: func(_ coordinateKeyPayload, destination, fragment coordinateScalarPayload, affected bool) (coordinateScalarPayload, bool) {
			if affected {
				return fragment, true
			}
			return destination, true
		},
		affectedSelector: func(builder *boundaryAffectedSelectorBuilder, key coordinateKeyPayload) {
			builder.anyIdentities(heapCoordinateKeyValue(key).id)
		},
		applyRootSkeleton: func(_ *boundaryApplyContext, skeleton coordinateSkeletonPayload, _ bool) (coordinateSkeletonPayload, bool) {
			return skeleton, true
		},
		rootSlot: func(_ *boundaryApplyContext, _ BoundaryFactorTarget) (coordinateKeyPayload, bool, bool) {
			return nil, false, true
		},
		rootScalar: func(_ *boundaryApplyContext, _ coordinateKeyPayload, _ product.Value) (coordinateScalarPayload, bool) {
			return nil, false
		},
	},
}

func heapCoordinateRootKey(id identity.Term) heapCoordinateKey {
	return heapCoordinateKey{kind: heapCoordinateRoot, id: id}
}

func heapCoordinateTopRootEntry(id identity.Term) coordinateEntry {
	return coordinateEntry{
		key:    wrapHeapCoordinateKey(heapCoordinateRootKey(id)),
		scalar: wrapHeapCoordinateScalar(product.Top()),
	}
}

func heapCoordinateInventoryCompletion() coordinateInventoryCompletionLaw {
	return coordinateInventoryCompletionLaw{
		kind: coordinateInventoryCompletionConsequences,
		emit: func(keys *keyspace.KeySpace, payload coordinateKeyPayload, emit func(coordinateKeyPayload) bool) bool {
			if payload == nil {
				return false
			}
			key := heapCoordinateKeyValue(payload)
			if !heapCoordinateKeyValid(key, keys) {
				return false
			}
			switch key.kind {
			case heapCoordinateRoot:
				return true
			case heapCoordinateMember:
				return emit(wrapHeapCoordinateKey(heapCoordinateRootKey(key.id)))
			default:
				return false
			}
		},
	}
}

func buildHeapCoordinateFamily(reg *axis.Registry, _ DomainOptions) coordinateFamilyOps {
	return withSemanticSkeletonRepresentation(coordinateFamilyOps{
		branchRelation:      noCoordinateBranchRelation(),
		inventoryCompletion: heapCoordinateInventoryCompletion(),
		requiredScalarKeys: func(skeletonPayload coordinateSkeletonPayload) []coordinateKeyPayload {
			skeleton := heapCoordinateSkeletonValue(skeletonPayload)
			if skeleton.top {
				return nil
			}
			ids := sortedHeapSkeletonIdentities(skeleton.objects)
			out := make([]coordinateKeyPayload, 0)
			for _, id := range ids {
				object := skeleton.objects[id]
				if object.bottom {
					continue
				}
				out = append(out, wrapHeapCoordinateKey(heapCoordinateRootKey(id)))
				for _, key := range object.staticKeys {
					out = append(out, wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateMember, id: id, key: key}))
				}
			}
			return out
		},
		sealSkeletonInventory: func(skeletonPayload coordinateSkeletonPayload, admitted []coordinateKeyPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, bool) {
			skeleton := heapCoordinateSkeletonValue(skeletonPayload)
			if skeleton.keys != nil && skeleton.keys != keys || keys == nil || !keys.Valid() {
				return nil, nil, false
			}
			if skeleton.top {
				skeleton.keys = keys
				return wrapHeapCoordinateSkeleton(skeleton), nil, true
			}
			inventory := make(map[heapCoordinateKey]struct{}, len(admitted))
			for _, payload := range admitted {
				if payload == nil {
					return nil, nil, false
				}
				key := heapCoordinateKeyValue(payload)
				if !heapCoordinateKeyValid(key, keys) {
					return nil, nil, false
				}
				inventory[key] = struct{}{}
			}
			out := heapCoordinateSkeleton{keys: keys}
			var post []coordinateEntry
			for id, object := range skeleton.objects {
				if object.bottom {
					if out.objects == nil {
						out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton)
					}
					out.objects[id] = cloneHeapCoordinateObject(object)
					continue
				}
				rootKey := heapCoordinateRootKey(id)
				if _, ok := inventory[rootKey]; !ok {
					object = heapTableIdentityObjectSkeleton{dynamicIndexFactsTop: true}
					post = append(post, heapCoordinateTopRootEntry(id))
				} else {
					filtered := make([]keyspace.Key, 0, len(object.staticKeys))
					for _, key := range object.staticKeys {
						if _, ok := inventory[heapCoordinateKey{kind: heapCoordinateMember, id: id, key: key}]; ok {
							filtered = append(filtered, key)
						}
					}
					if len(filtered) != len(object.staticKeys) {
						object.staticKeys = filtered
						object.stableShape = false
						object.prefixStableShape = false
					}
				}
				if out.objects == nil {
					out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton)
				}
				out.objects[id] = cloneHeapCoordinateObject(object)
			}
			sort.Slice(post, func(i, j int) bool {
				return heapCoordinateKeyLess(heapCoordinateKeyValue(post[i].key), heapCoordinateKeyValue(post[j].key), keys)
			})
			return wrapHeapCoordinateSkeleton(out), post, true
		},
		selectionSupport: func(skeletonPayload coordinateSkeletonPayload, selected []coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			skeleton := heapCoordinateSkeletonValue(skeletonPayload)
			if skeleton.top {
				return nil, true
			}
			objects := make(map[identity.Term]struct{}, len(selected))
			for _, payload := range selected {
				key := heapCoordinateKeyValue(payload)
				if !heapCoordinateKeyValid(key, skeleton.keys) {
					return nil, false
				}
				if _, exists := skeleton.objects[key.id]; exists {
					objects[key.id] = struct{}{}
				}
			}
			out := make([]coordinateKeyPayload, 0, len(selected))
			for _, id := range sortedHeapSkeletonIdentities(skeleton.objects) {
				if _, selected := objects[id]; !selected || skeleton.objects[id].bottom {
					continue
				}
				out = append(out, wrapHeapCoordinateKey(heapCoordinateRootKey(id)))
				for _, key := range skeleton.objects[id].staticKeys {
					out = append(out, wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateMember, id: id, key: key}))
				}
			}
			return out, true
		},
		sealSelectedSkeletonOverlay: func(selected []coordinateKeyPayload, _ *keyspace.KeySpace) (coordinateSkeletonOverlayPlanPayload, bool) {
			plan := heapCoordinateOverlayPlan{selectedCount: len(selected), byObject: make(map[identity.Term]*heapSelectedSkeletonObject)}
			for _, payload := range selected {
				key := heapCoordinateKeyValue(payload)
				entry := plan.byObject[key.id]
				if entry == nil {
					entry = &heapSelectedSkeletonObject{}
					plan.byObject[key.id] = entry
				}
				switch key.kind {
				case heapCoordinateRoot:
					entry.root = true
				case heapCoordinateMember:
					entry.members = append(entry.members, key.key)
				default:
					return nil, false
				}
			}
			return typedCoordinateSkeletonOverlayPlanPayload[heapCoordinateOverlayPlan]{value: plan}, true
		},
		overlaySelectedSkeleton: func(payload coordinateSkeletonOverlayPlanPayload, current, image coordinateSkeletonPayload, _ []CoordinateScalarFactor, keys *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			typed, ok := payload.(typedCoordinateSkeletonOverlayPlanPayload[heapCoordinateOverlayPlan])
			if !ok {
				return nil, false
			}
			out, ok := overlaySelectedHeapCoordinateSkeleton(
				heapCoordinateSkeletonValue(current), heapCoordinateSkeletonValue(image), typed.value, keys,
			)
			return wrapHeapCoordinateSkeleton(out), ok
		},
		decompose: func(payload laneFactorPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, error) {
			if keys == nil || !keys.Valid() {
				return nil, nil, fmt.Errorf("heap coordinates require a valid keyspace")
			}
			lane := typedLaneFactorValue[heapTableIdentityLane](payload)
			skeleton := heapCoordinateSkeleton{keys: keys, top: lane.top}
			if lane.top {
				return wrapHeapCoordinateSkeleton(skeleton), nil, nil
			}
			entries := make([]coordinateEntry, 0)
			if len(lane.values) != 0 {
				skeleton.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(lane.values))
			}
			for id, object := range lane.values {
				if !id.Valid() {
					return nil, nil, fmt.Errorf("heap has an empty identity")
				}
				metadata := heapObjectSkeletonFromObject(keys, object)
				skeleton.objects[id] = metadata
				if object.IsBottom() {
					continue
				}
				entries = append(entries, coordinateEntry{key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateRoot, id: id}), scalar: wrapHeapCoordinateScalar(object.Root())})
				object.VisitStaticMembers(func(key keyspace.Key, value product.Value) bool {
					entries = append(entries, coordinateEntry{key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateMember, id: id, key: key}), scalar: wrapHeapCoordinateScalar(value)})
					return true
				})
			}
			sort.Slice(entries, func(i, j int) bool {
				return heapCoordinateKeyLess(heapCoordinateKeyValue(entries[i].key), heapCoordinateKeyValue(entries[j].key), keys)
			})
			return wrapHeapCoordinateSkeleton(skeleton), entries, nil
		},
		replace: func(_ laneFactorPayload, keys *keyspace.KeySpace, skeletonPayload coordinateSkeletonPayload, entries []coordinateEntry) (laneFactorPayload, error) {
			skeleton := heapCoordinateSkeletonValue(skeletonPayload)
			if keys == nil || !keys.Valid() || skeleton.keys != nil && skeleton.keys != keys {
				return nil, fmt.Errorf("heap coordinate keyspace mismatch")
			}
			if skeleton.top {
				if len(entries) != 0 {
					return nil, fmt.Errorf("heap Top cannot carry coordinates")
				}
				return typedLaneFactorPayload[heapTableIdentityLane]{value: heapTableIdentityLane{top: true}}, nil
			}
			roots := make(map[identity.Term]product.Value)
			members := make(map[identity.Term]map[keyspace.Key]product.Value)
			for index, entry := range entries {
				key := heapCoordinateKeyValue(entry.key)
				value := heapCoordinateScalarValue(entry.scalar).value
				if !heapCoordinateKeyValid(key, keys) || !product.BelongsToRegistry(reg, value) {
					return nil, fmt.Errorf("invalid heap coordinate %d", index)
				}
				object, exists := skeleton.objects[key.id]
				if !exists || object.bottom {
					return nil, fmt.Errorf("heap coordinate %d has no object", index)
				}
				switch key.kind {
				case heapCoordinateRoot:
					if _, duplicate := roots[key.id]; duplicate {
						return nil, fmt.Errorf("duplicate heap root")
					}
					roots[key.id] = value
				case heapCoordinateMember:
					if !sortedHeapKeyContains(keys, object.staticKeys, key.key) {
						return nil, fmt.Errorf("heap member is absent from skeleton")
					}
					if members[key.id] == nil {
						members[key.id] = make(map[keyspace.Key]product.Value)
					}
					if _, duplicate := members[key.id][key.key]; duplicate {
						return nil, fmt.Errorf("duplicate heap member")
					}
					members[key.id][key.key] = value
				}
			}
			objects := make(map[identity.Term]heapidentity.TableObject, len(skeleton.objects))
			for id, metadata := range skeleton.objects {
				if metadata.bottom {
					objects[id] = heapidentity.BottomObject(reg)
					continue
				}
				root, ok := roots[id]
				if !ok || len(members[id]) != len(metadata.staticKeys) {
					return nil, fmt.Errorf("incomplete heap object %v", id)
				}
				object, err := recomposeHeapTableIdentityObject(reg, keys, metadata, root, members[id])
				if err != nil {
					return nil, err
				}
				objects[id] = object
			}
			lane := heapTableIdentityLaneFromMap(heapTermMapDomain(reg), objects)
			return typedLaneFactorPayload[heapTableIdentityLane]{value: lane}, nil
		},

		skeletonBottom: func() coordinateSkeletonPayload { return wrapHeapCoordinateSkeleton(heapCoordinateSkeleton{}) },
		skeletonTop:    func() coordinateSkeletonPayload { return wrapHeapCoordinateSkeleton(heapCoordinateSkeleton{top: true}) },
		skeletonEqual: func(a, b coordinateSkeletonPayload) bool {
			left, right := heapCoordinateSkeletonValue(a), heapCoordinateSkeletonValue(b)
			return heapCoordinateSkeletonLessOrEq(reg, left, right) && heapCoordinateSkeletonLessOrEq(reg, right, left)
		},
		skeletonLessOrEq: func(a, b coordinateSkeletonPayload) bool {
			return heapCoordinateSkeletonLessOrEq(reg, heapCoordinateSkeletonValue(a), heapCoordinateSkeletonValue(b))
		},
		skeletonJoin: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapHeapCoordinateSkeleton(joinHeapCoordinateSkeleton(reg, heapCoordinateSkeletonValue(a), heapCoordinateSkeletonValue(b), false))
		},
		skeletonMeet: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapHeapCoordinateSkeleton(meetHeapCoordinateSkeleton(reg, heapCoordinateSkeletonValue(a), heapCoordinateSkeletonValue(b)))
		},
		skeletonWiden: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapHeapCoordinateSkeleton(joinHeapCoordinateSkeleton(reg, heapCoordinateSkeletonValue(a), heapCoordinateSkeletonValue(b), true))
		},
		skeletonNarrow: func(previous, _ coordinateSkeletonPayload) coordinateSkeletonPayload { return previous },
		skeletonHash: func(value coordinateSkeletonPayload) uint64 {
			return hashHeapCoordinateSkeleton(reg, heapCoordinateSkeletonValue(value))
		},
		importSkeleton: func(source coordinateSkeletonPayload, from, to *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			value, ok := importHeapCoordinateSkeleton(reg, heapCoordinateSkeletonValue(source), from, to)
			return wrapHeapCoordinateSkeleton(value), ok
		},

		keyValid: func(key coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return heapCoordinateKeyValid(heapCoordinateKeyValue(key), keys)
		},
		keyEqual: func(a, b coordinateKeyPayload) bool {
			return heapCoordinateKeyValue(a) == heapCoordinateKeyValue(b)
		},
		keyLess: func(a, b coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return heapCoordinateKeyLess(heapCoordinateKeyValue(a), heapCoordinateKeyValue(b), keys)
		},
		keyHash: func(key coordinateKeyPayload, _ *keyspace.KeySpace) uint64 {
			return hashHeapCoordinateKey(heapCoordinateKeyValue(key))
		},
		importKey: func(source coordinateKeyPayload, from, to *keyspace.KeySpace) (coordinateKeyPayload, bool) {
			key := heapCoordinateKeyValue(source)
			if key.kind == heapCoordinateMember {
				var ok bool
				key.key, ok = to.ImportKey(from, key.key)
				if !ok {
					return nil, false
				}
			}
			return wrapHeapCoordinateKey(key), heapCoordinateKeyValid(key, to)
		},
		formalRekey: coordinateFormalRekeyPolicy{
			kind: coordinateFormalRekeyStructural,
			skeleton: func(source coordinateSkeletonPayload, plan CoordinateFormalRootRekey) (coordinateSkeletonPayload, bool) {
				value, ok := mapHeapCoordinateSkeletonKeys(reg, heapCoordinateSkeletonValue(source), plan.to, plan.rekey)
				return wrapHeapCoordinateSkeleton(value), ok
			},
			key: func(source coordinateKeyPayload, plan CoordinateFormalRootRekey) (coordinateKeyPayload, bool) {
				key := heapCoordinateKeyValue(source)
				if key.kind == heapCoordinateMember {
					var ok bool
					key.key, ok = plan.rekey(key.key)
					if !ok {
						return nil, false
					}
				}
				return wrapHeapCoordinateKey(key), heapCoordinateKeyValid(key, plan.to)
			},
		},
		visitValueDependencies: func(coordinateKeyPayload, *keyspace.KeySpace, func(statekey.ValueDependency)) {},

		defaultScalar: func(skeleton coordinateSkeletonPayload, keyPayload coordinateKeyPayload) (coordinateScalarPayload, error) {
			value := heapCoordinateSkeletonValue(skeleton)
			key := heapCoordinateKeyValue(keyPayload)
			if value.top {
				return wrapHeapCoordinateScalar(product.Top()), nil
			}
			object, exists := value.objects[key.id]
			if !exists || object.bottom {
				return wrapHeapCoordinateScalar(product.Bottom(reg)), nil
			}
			if key.kind == heapCoordinateMember && !sortedHeapKeyContains(value.keys, object.staticKeys, key.key) {
				return wrapHeapCoordinateScalar(product.Top()), nil
			}
			return nil, fmt.Errorf("explicit heap coordinate has no omitted default")
		},
		scalarSupport: func(skeleton coordinateSkeletonPayload, keyPayload coordinateKeyPayload) CoordinateScalarSupport {
			value := heapCoordinateSkeletonValue(skeleton)
			key := heapCoordinateKeyValue(keyPayload)
			if value.top {
				return CoordinateScalarForbidden
			}
			object, exists := value.objects[key.id]
			if !exists || object.bottom {
				return CoordinateScalarForbidden
			}
			if key.kind == heapCoordinateRoot || key.kind == heapCoordinateMember && sortedHeapKeyContains(value.keys, object.staticKeys, key.key) {
				return CoordinateScalarRequired
			}
			return CoordinateScalarForbidden
		},
		scalarValid: func(_ coordinateKeyPayload, scalar coordinateScalarPayload) bool {
			return product.BelongsToRegistry(reg, heapCoordinateScalarValue(scalar).value)
		},
		scalarEqual: func(a, b coordinateScalarPayload) bool {
			return product.Equal(reg, heapCoordinateScalarValue(a).value, heapCoordinateScalarValue(b).value)
		},
		scalarLessOrEq: func(a, b coordinateScalarPayload) bool {
			return product.LessOrEq(reg, heapCoordinateScalarValue(a).value, heapCoordinateScalarValue(b).value)
		},
		scalarJoin: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapHeapCoordinateScalar(product.Join(reg, heapCoordinateScalarValue(a).value, heapCoordinateScalarValue(b).value))
		},
		scalarMeet: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapHeapCoordinateScalar(product.Meet(reg, heapCoordinateScalarValue(a).value, heapCoordinateScalarValue(b).value))
		},
		scalarWiden: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapHeapCoordinateScalar(product.Widen(reg, heapCoordinateScalarValue(a).value, heapCoordinateScalarValue(b).value))
		},
		scalarNarrow: func(previous, _ coordinateScalarPayload) coordinateScalarPayload { return previous },
		scalarHash: func(value coordinateScalarPayload) uint64 {
			return product.Hash(reg, heapCoordinateScalarValue(value).value)
		},
		importScalar: func(source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			value := heapCoordinateScalarValue(source).value
			return wrapHeapCoordinateScalar(value), product.BelongsToRegistry(reg, value)
		},
		returnIdentity: coordinateReturnIdentityOps{
			roles: coordinateReturnIdentityRoles(
				CoordinateReturnIdentitySeed,
				CoordinateReturnIdentitySkeletonEdge,
				CoordinateReturnIdentityScalarEdge,
				CoordinateReturnIdentityContainer,
			),
			visitInventoryTerms: func(payload coordinateKeyPayload, visit func(identity.Term) bool) bool {
				term := heapCoordinateKeyValue(payload).id
				return !term.Valid() || visit(term)
			},
			visitTermKeys: func(identity.Term, func(coordinateKeyPayload) bool) bool { return true },
			imageInventoryKey: func(payload coordinateKeyPayload, image *CoordinateIdentityTermImage, emit func(coordinateKeyPayload) bool) bool {
				key := heapCoordinateKeyValue(payload)
				terms, ok := image.Image(key.id)
				if !ok {
					return false
				}
				for _, term := range terms {
					mapped := key
					mapped.id = term
					if !emit(wrapHeapCoordinateKey(mapped)) {
						return false
					}
				}
				return true
			},
			visitSeedRoots: func(source coordinateSkeletonPayload, visit func(identity.Term) bool) {
				skeleton := heapCoordinateSkeletonValue(source)
				if skeleton.top {
					return
				}
				for _, term := range sortedHeapSkeletonIdentities(skeleton.objects) {
					if !skeleton.objects[term].bottom && !visit(term) {
						return
					}
				}
			},
			visitSkeletonEdges: func(source coordinateSkeletonPayload, visit func(identity.Term, identity.Term) bool) {
				skeleton := heapCoordinateSkeletonValue(source)
				if skeleton.top {
					return
				}
				for _, fromTerm := range sortedHeapSkeletonIdentities(skeleton.objects) {
					object := skeleton.objects[fromTerm]
					for _, dynamicKey := range sortedHeapDynamicKeys(skeleton.keys, object.dynamicIndexFacts) {
						fact := object.dynamicIndexFacts[dynamicKey]
						for _, value := range []product.Value{fact.KeyValue, fact.Value} {
							if to, exact := product.Get(reg, value, identity.Key).Term(); exact && to.Valid() {
								if !visit(fromTerm, to) {
									return
								}
							}
						}
					}
				}
			},
			visitScalarEdges: func(keyPayload coordinateKeyPayload, scalar coordinateScalarPayload, visit func(identity.Term, identity.Term) bool) {
				key := heapCoordinateKeyValue(keyPayload)
				to, exact := product.Get(reg, heapCoordinateScalarValue(scalar).value, identity.Key).Term()
				if exact && to.Valid() && key.id.Valid() {
					visit(key.id, to)
				}
			},
			containerScalar: func(keyPayload coordinateKeyPayload, scalar coordinateScalarPayload) (identity.Term, product.Value, bool) {
				key := heapCoordinateKeyValue(keyPayload)
				if key.kind != heapCoordinateRoot {
					return identity.Term{}, product.Value{}, false
				}
				return key.id, heapCoordinateScalarValue(scalar).value, key.id.Valid()
			},
			visitContainerFacts: func(source coordinateSkeletonPayload, term identity.Term, visit func(dynamicindex.Fact)) bool {
				skeleton := heapCoordinateSkeletonValue(source)
				if skeleton.top {
					return false
				}
				object, present := skeleton.objects[term]
				if !present || object.bottom {
					return false
				}
				for _, dynamicKey := range sortedHeapDynamicKeys(skeleton.keys, object.dynamicIndexFacts) {
					visit(object.dynamicIndexFacts[dynamicKey])
				}
				return true
			},
			publicationKey: func(identity.Term) (coordinateKeyPayload, bool) { return nil, false },
			publishScalar: func(coordinateKeyPayload, coordinateScalarPayload, placement.Value) (coordinateScalarPayload, bool) {
				return nil, false
			},
		},
		pathEvidence: noCoordinatePathEvidence(),
		pathValues:   noCoordinatePathValues(),
		rootAssignment: uniqueCoordinateRootAssignment(func(source coordinateSkeletonPayload, id identity.ID) bool {
			skeleton := heapCoordinateSkeletonValue(source)
			if skeleton.top {
				return false
			}
			object, present := skeleton.objects[identity.ConcreteTerm(id)]
			return present && !object.bottom && len(object.staticKeys) == 0 &&
				!object.dynamicIndexFactsTop && len(object.dynamicIndexFacts) == 0
		}),
		pathMutation:   noCoordinatePathMutation(),
		objectMutation: heapCoordinateObjectMutation(reg),
	})
}

func heapObjectSkeletonFromObject(keys *keyspace.KeySpace, object heapidentity.TableObject) heapTableIdentityObjectSkeleton {
	out := heapTableIdentityObjectSkeleton{
		bottom: object.IsBottom(), dynamicIndexFactsTop: object.DynamicIndexFactsTop(),
		stableShape: object.StableShape(), prefixStableShape: object.PrefixStableShape(),
	}
	if out.bottom {
		return out
	}
	out.dynamicIndexFacts = object.DynamicIndexFacts()
	object.VisitStaticMembers(func(key keyspace.Key, _ product.Value) bool {
		out.staticKeys = append(out.staticKeys, key)
		return true
	})
	sort.Slice(out.staticKeys, func(i, j int) bool { return keys.Less(out.staticKeys[i], out.staticKeys[j]) })
	return out
}

func heapCoordinateSkeletonLessOrEq(reg *axis.Registry, left, right heapCoordinateSkeleton) bool {
	if left.top {
		return right.top
	}
	if right.top {
		return true
	}
	keys := left.keys
	if keys == nil {
		keys = right.keys
	}
	for id, leftObject := range left.objects {
		rightObject, ok := right.objects[id]
		if !ok {
			if !leftObject.bottom {
				return false
			}
			continue
		}
		if !heapObjectSkeletonLessOrEq(reg, keys, leftObject, rightObject) {
			return false
		}
	}
	return true
}

func joinHeapCoordinateSkeleton(reg *axis.Registry, left, right heapCoordinateSkeleton, widen bool) heapCoordinateSkeleton {
	keys := left.keys
	if keys == nil {
		keys = right.keys
	}
	if left.top || right.top {
		return heapCoordinateSkeleton{keys: keys, top: true}
	}
	out := heapCoordinateSkeleton{keys: keys, objects: make(map[identity.Term]heapTableIdentityObjectSkeleton, len(left.objects)+len(right.objects))}
	for id, object := range left.objects {
		out.objects[id] = cloneHeapCoordinateObject(object)
	}
	for id, rightObject := range right.objects {
		leftObject, exists := left.objects[id]
		switch {
		case !exists || leftObject.bottom:
			if !rightObject.bottom {
				out.objects[id] = cloneHeapCoordinateObject(rightObject)
			}
		case rightObject.bottom:
		default:
			out.objects[id] = joinHeapCoordinateObject(reg, keys, leftObject, rightObject, widen)
		}
	}
	if len(out.objects) == 0 {
		out.objects = nil
	}
	return out
}

func meetHeapCoordinateSkeleton(reg *axis.Registry, left, right heapCoordinateSkeleton) heapCoordinateSkeleton {
	keys := left.keys
	if keys == nil {
		keys = right.keys
	}
	if left.top {
		out := cloneHeapCoordinateSkeleton(right)
		out.keys = keys
		return out
	}
	if right.top {
		out := cloneHeapCoordinateSkeleton(left)
		out.keys = keys
		return out
	}
	out := heapCoordinateSkeleton{keys: keys}
	for id, leftObject := range left.objects {
		rightObject, exists := right.objects[id]
		if !exists || leftObject.bottom || rightObject.bottom {
			continue
		}
		if out.objects == nil {
			out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton)
		}
		out.objects[id] = meetHeapObjectSkeleton(reg, keys, leftObject, rightObject)
	}
	return out
}

func joinHeapCoordinateObject(reg *axis.Registry, keys *keyspace.KeySpace, left, right heapTableIdentityObjectSkeleton, widen bool) heapTableIdentityObjectSkeleton {
	return joinHeapObjectSkeleton(reg, keys, left, right, widen)
}

func cloneHeapCoordinateObject(source heapTableIdentityObjectSkeleton) heapTableIdentityObjectSkeleton {
	out := source
	out.staticKeys = append([]keyspace.Key(nil), source.staticKeys...)
	out.dynamicIndexFacts = dynamicindex.CloneMap(source.dynamicIndexFacts)
	return out
}

func cloneHeapCoordinateSkeleton(source heapCoordinateSkeleton) heapCoordinateSkeleton {
	out := heapCoordinateSkeleton{keys: source.keys, top: source.top}
	if len(source.objects) != 0 {
		out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(source.objects))
		for id, object := range source.objects {
			out.objects[id] = cloneHeapCoordinateObject(object)
		}
	}
	return out
}

type heapSelectedSkeletonObject struct {
	root    bool
	members []keyspace.Key
}

// overlaySelectedHeapCoordinateSkeleton overlays the object-existence/root
// skeleton and exact static-member support owned by selected. Object metadata
// belongs to the selected root; member support remains independently keyed.
// A root cannot be removed while an unselected member remains supported,
// because such a mixed skeleton has no representation in the heap family.
func overlaySelectedHeapCoordinateSkeleton(
	current, image heapCoordinateSkeleton,
	plan heapCoordinateOverlayPlan,
	keys *keyspace.KeySpace,
) (heapCoordinateSkeleton, bool) {
	if plan.selectedCount == 0 || current.top {
		out := cloneHeapCoordinateSkeleton(current)
		out.keys = keys
		return out, true
	}
	out := cloneHeapCoordinateSkeleton(current)
	out.keys = keys
	for id, selection := range plan.byObject {
		currentObject, hasCurrent := current.objects[id]
		imageObject, hasImage := image.objects[id]
		currentSupported := hasCurrent && !currentObject.bottom
		imageSupported := hasImage && !imageObject.bottom

		if !selection.root && currentSupported != imageSupported {
			// The root is unselected, so member selection cannot change object
			// existence as a side effect.
			return heapCoordinateSkeleton{}, false
		}
		if selection.root && !imageSupported {
			if currentSupported {
				for _, key := range currentObject.staticKeys {
					if !sortedHeapKeyContains(keys, selection.members, key) {
						return heapCoordinateSkeleton{}, false
					}
				}
			}
			if out.objects != nil {
				if hasImage {
					out.objects[id] = cloneHeapCoordinateObject(imageObject)
				} else {
					delete(out.objects, id)
				}
			}
			continue
		}
		if !currentSupported && !selection.root {
			for _, key := range selection.members {
				if imageSupported && sortedHeapKeyContains(keys, imageObject.staticKeys, key) {
					return heapCoordinateSkeleton{}, false
				}
			}
			continue
		}

		var object heapTableIdentityObjectSkeleton
		switch {
		case selection.root:
			object = cloneHeapCoordinateObject(imageObject)
		case hasCurrent:
			object = cloneHeapCoordinateObject(currentObject)
		default:
			continue
		}
		object.staticKeys = overlayHeapStaticKeys(keys, currentObject.staticKeys, imageObject.staticKeys, selection.members)
		if out.objects == nil {
			out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton)
		}
		out.objects[id] = object
	}
	if len(out.objects) == 0 {
		out.objects = nil
	}
	return out, true
}

func overlayHeapStaticKeys(keys *keyspace.KeySpace, current, image, selected []keyspace.Key) []keyspace.Key {
	return overlaySelectedKeyspaceKeys(keys, current, image, selected)
}

func mapHeapCoordinateObjectValues(source heapTableIdentityObjectSkeleton, mapValue func(product.Value) (product.Value, bool)) heapTableIdentityObjectSkeleton {
	out := cloneHeapCoordinateObject(source)
	if out.bottom || out.dynamicIndexFactsTop {
		return out
	}
	for key, fact := range out.dynamicIndexFacts {
		var ok bool
		fact.KeyValue, ok = mapValue(fact.KeyValue)
		if !ok {
			return out
		}
		fact.Value, ok = mapValue(fact.Value)
		if !ok {
			return out
		}
		out.dynamicIndexFacts[key] = fact
	}
	return out
}

func importHeapCoordinateObjectKeys(reg *axis.Registry, source heapTableIdentityObjectSkeleton, from, to *keyspace.KeySpace) (heapTableIdentityObjectSkeleton, bool) {
	if from == nil || to == nil || !from.Valid() || !to.Valid() {
		return heapTableIdentityObjectSkeleton{}, false
	}
	return mapHeapCoordinateObjectKeys(reg, source, to, func(source keyspace.Key) (keyspace.Key, bool) { return to.ImportKey(from, source) })
}

func mapHeapCoordinateObjectKeys(reg *axis.Registry, source heapTableIdentityObjectSkeleton, to *keyspace.KeySpace, mapKey func(keyspace.Key) (keyspace.Key, bool)) (heapTableIdentityObjectSkeleton, bool) {
	if to == nil || !to.Valid() || mapKey == nil {
		return heapTableIdentityObjectSkeleton{}, false
	}
	out := cloneHeapCoordinateObject(source)
	for index, key := range out.staticKeys {
		var ok bool
		out.staticKeys[index], ok = mapKey(key)
		if !ok {
			return heapTableIdentityObjectSkeleton{}, false
		}
	}
	sort.Slice(out.staticKeys, func(i, j int) bool { return to.Less(out.staticKeys[i], out.staticKeys[j]) })
	if len(out.dynamicIndexFacts) != 0 {
		rekeyed := make(map[dynamicindex.Key]dynamicindex.Fact, len(out.dynamicIndexFacts))
		for key, fact := range out.dynamicIndexFacts {
			table, ok := mapKey(key.Table)
			if !ok {
				return heapTableIdentityObjectSkeleton{}, false
			}
			next := dynamicindex.Key{Table: table, Site: key.Site}
			if existing, collision := rekeyed[next]; collision {
				fact = dynamicindex.Domain(reg).Join(existing, fact)
			}
			rekeyed[next] = fact
		}
		out.dynamicIndexFacts = rekeyed
	}
	return out, true
}

func mapHeapCoordinateSkeletonKeys(reg *axis.Registry, source heapCoordinateSkeleton, to *keyspace.KeySpace, mapKey func(keyspace.Key) (keyspace.Key, bool)) (heapCoordinateSkeleton, bool) {
	if to == nil || !to.Valid() || mapKey == nil {
		return heapCoordinateSkeleton{}, false
	}
	out := heapCoordinateSkeleton{keys: to, top: source.top}
	if source.top {
		return out, true
	}
	if len(source.objects) != 0 {
		out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(source.objects))
	}
	for id, object := range source.objects {
		mapped, ok := mapHeapCoordinateObjectKeys(reg, object, to, mapKey)
		if !ok {
			return heapCoordinateSkeleton{}, false
		}
		out.objects[id] = mapped
	}
	return out, true
}

func importHeapCoordinateSkeleton(reg *axis.Registry, source heapCoordinateSkeleton, from, to *keyspace.KeySpace) (heapCoordinateSkeleton, bool) {
	if to == nil || !to.Valid() {
		return heapCoordinateSkeleton{}, false
	}
	if source.keys == nil && len(source.objects) == 0 {
		source.keys = to
		return source, true
	}
	if from == nil {
		from = source.keys
	}
	out := heapCoordinateSkeleton{keys: to, top: source.top}
	if source.top {
		return out, true
	}
	if len(source.objects) != 0 {
		out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(source.objects))
	}
	for id, object := range source.objects {
		var ok bool
		out.objects[id], ok = importHeapCoordinateObjectKeys(reg, object, from, to)
		if !ok {
			return heapCoordinateSkeleton{}, false
		}
	}
	return out, true
}

func heapCoordinateKeyValid(key heapCoordinateKey, keys *keyspace.KeySpace) bool {
	if !key.id.Valid() || keys == nil || !keys.Valid() {
		return false
	}
	switch key.kind {
	case heapCoordinateRoot:
		return key.key == (keyspace.Key{})
	case heapCoordinateMember:
		// Heap objects retain the constructor's exact member key. Most members
		// are rootless suffixes, while imported/visible object facts may carry a
		// rooted structural key. Both are valid coordinates when they own at
		// least one immutable member segment; formal rekey substitutes the root
		// only for the latter.
		segments, ok := keys.SegmentsView(key.key)
		return ok && len(segments) != 0
	default:
		return false
	}
}

func heapCoordinateKeyLess(left, right heapCoordinateKey, keys *keyspace.KeySpace) bool {
	if left.id != right.id {
		return identityTermLess(left.id, right.id)
	}
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	return left.kind == heapCoordinateMember && left.key != right.key && keys.Less(left.key, right.key)
}

func hashHeapCoordinateKey(key heapCoordinateKey) uint64 {
	h := internal.MixHash(internal.FnvString("heap.coordinate"), uint64(key.kind))
	h = internal.MixHash(h, key.id.Hash())
	if key.kind == heapCoordinateMember {
		h = hashStructuralPathKey(h, key.key)
	}
	return h
}

func hashStructuralPathKey(seed uint64, key keyspace.Key) uint64 {
	seed = internal.MixHash(seed, uint64(key.Kind))
	seed = internal.MixHash(seed, uint64(key.Sym))
	seed = internal.MixHash(seed, uint64(key.Ver))
	seed = internal.MixHash(seed, uint64(key.Root))
	seed = internal.MixHash(seed, uint64(key.Segs))
	if key.Canon {
		seed = internal.MixHash(seed, 1)
	}
	return seed
}

func hashHeapCoordinateSkeleton(reg *axis.Registry, skeleton heapCoordinateSkeleton) uint64 {
	h := internal.FnvString("heap.coordinate.skeleton")
	if skeleton.top {
		return internal.MixHash(h, 1)
	}
	for _, id := range sortedHeapSkeletonIdentities(skeleton.objects) {
		object := skeleton.objects[id]
		if object.bottom {
			continue
		}
		h = internal.MixHash(h, id.Hash())
		for _, key := range object.staticKeys {
			h = hashStructuralPathKey(h, key)
		}
		if object.dynamicIndexFactsTop {
			h = internal.MixHash(h, 1)
		} else {
			for _, key := range sortedHeapDynamicKeys(skeleton.keys, object.dynamicIndexFacts) {
				fact := object.dynamicIndexFacts[key]
				h = hashStructuralPathKey(h, key.Table)
				h = internal.MixHash(h, internal.FnvString(string(key.Site)))
				h = internal.MixHash(h, uint64(fact.KeyPresence))
				h = internal.MixHash(h, product.Hash(reg, fact.KeyValue))
				h = internal.MixHash(h, product.Hash(reg, fact.Value))
				h = internal.MixHash(h, uint64(fact.Admission))
			}
		}
		if object.stableShape {
			h = internal.MixHash(h, 2)
		}
		if object.prefixStableShape {
			h = internal.MixHash(h, 4)
		}
	}
	return h
}

func heapCoordinateSkeletonValue(payload coordinateSkeletonPayload) heapCoordinateSkeleton {
	typed, ok := payload.(typedCoordinateSkeletonPayload[heapCoordinateSkeleton])
	if !ok {
		panic("state: heap coordinate skeleton mismatch")
	}
	return typed.value
}

func heapCoordinateKeyValue(payload coordinateKeyPayload) heapCoordinateKey {
	typed, ok := payload.(typedCoordinateKeyPayload[heapCoordinateKey])
	if !ok {
		panic("state: heap coordinate key mismatch")
	}
	return typed.value
}

func heapCoordinateScalarValue(payload coordinateScalarPayload) heapCoordinateScalar {
	typed, ok := payload.(typedCoordinateScalarPayload[heapCoordinateScalar])
	if !ok {
		panic("state: heap coordinate scalar mismatch")
	}
	return typed.value
}

func wrapHeapCoordinateSkeleton(value heapCoordinateSkeleton) coordinateSkeletonPayload {
	return typedCoordinateSkeletonPayload[heapCoordinateSkeleton]{value: value}
}

func wrapHeapCoordinateKey(value heapCoordinateKey) coordinateKeyPayload {
	return typedCoordinateKeyPayload[heapCoordinateKey]{value: value}
}

func wrapHeapCoordinateScalar(value product.Value) coordinateScalarPayload {
	return typedCoordinateScalarPayload[heapCoordinateScalar]{value: heapCoordinateScalar{value: value}}
}
