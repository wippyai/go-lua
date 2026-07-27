package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/lift"
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

const lenFloorCoordinateFamilyID CoordinateFamilyID = "path-length-floor"

type lenFloorCoordinateSkeleton struct {
	bottom bool
	keys   *keyspace.KeySpace
	paths  []keyspace.Key
}
type lenFloorCoordinateKey struct{ path keyspace.Key }
type lenFloorCoordinateScalar struct{ floor lenbound.Floor }
type lenFloorCoordinateOverlayPlan []keyspace.Key

var lenFloorCoordinateFamilySpec = coordinateFamilySpec{
	dynamicRead:   dynamicReadLenFloorCoordinates(),
	identityImage: IdentityImageIndependent,
	id:            lenFloorCoordinateFamilyID,
	build:         buildLenFloorCoordinateFamily,
	boundary: coordinateFamilyBoundaryOps{
		admission: coordinateBoundaryAdmissionAllPreimages,
		rootUse:   boundaryRootUseReachability(),
		reachabilityKey: func(program *boundaryReachabilityProgramBuilder, source coordinateKeyPayload) {
			program.pathCone(false, lenFloorCoordinateKeyValue(source).path)
		},
		projectSkeleton: func(ctx *boundaryProjectContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			value := cloneLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(source))
			if value.bottom {
				return wrapLenFloorCoordinateSkeleton(value), true
			}
			value.paths = filterLenFloorPaths(value.paths, ctx.closure.ContainsPath)
			return wrapLenFloorCoordinateSkeleton(value), true
		},
		projectKey: func(ctx *boundaryProjectContext, source coordinateKeyPayload) (coordinateKeyPayload, bool, bool) {
			return source, ctx.closure.ContainsPath(lenFloorCoordinateKeyValue(source).path), true
		},
		projectScalar: func(_ *boundaryProjectContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		rebaseSkeleton: func(ctx *boundaryRebaseContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			value := lenFloorCoordinateSkeletonValue(source)
			if value.bottom {
				value.keys = ctx.toKeys
				return wrapLenFloorCoordinateSkeleton(value), true
			}
			mapped, ok := rebaseBoundaryMustSet(sliceKeySet(value.paths), func(path keyspace.Key) ([]keyspace.Key, bool) { return boundaryRebasePaths(ctx, path) }, func(path keyspace.Key) keyspace.Key { return path }, func(path keyspace.Key) ([]keyspace.Key, bool) { return ctx.quotient.pathPreimages(path) })
			if !ok {
				return nil, false
			}
			paths := sortedLenFloorPathSet(ctx.toKeys, mapped)
			return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{keys: ctx.toKeys, paths: paths}), true
		},
		rebaseKeys: func(ctx *boundaryRebaseContext, source coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			paths, ok := boundaryRebasePaths(ctx, lenFloorCoordinateKeyValue(source).path)
			if !ok {
				return nil, false
			}
			out := make([]coordinateKeyPayload, len(paths))
			for index, path := range paths {
				out[index] = wrapLenFloorCoordinateKey(path)
			}
			return out, true
		},
		rebaseScalar: func(_ *boundaryRebaseContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		sourceFiber: func(source coordinateKeyPayload) coordinateFiberPayload {
			return typedCoordinateFiberPayload[keyspace.Key]{value: lenFloorCoordinateKeyValue(source).path}
		},
		inverseFibers: func(ctx *boundaryRebaseContext, destination coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
			paths, ok := ctx.quotient.pathPreimages(lenFloorCoordinateKeyValue(destination).path)
			if !ok {
				return nil, false
			}
			out := make([]coordinateFiberPayload, len(paths))
			for index, path := range paths {
				out[index] = typedCoordinateFiberPayload[keyspace.Key]{value: path}
			}
			return out, true
		},
		postEntries: noCoordinatePostEntries,
		applySkeleton: func(ctx *boundaryApplyContext, destination, fragment coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			left, right := lenFloorCoordinateSkeletonValue(destination), lenFloorCoordinateSkeletonValue(fragment)
			if left.bottom || right.bottom {
				return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{bottom: true, keys: left.keys}), true
			}
			values := sliceKeySet(left.paths)
			for path := range values {
				if ctx.closure.ContainsPath(path) {
					delete(values, path)
				}
			}
			for _, path := range right.paths {
				values[path] = struct{}{}
			}
			paths := sortedLenFloorPathSet(left.keys, values)
			return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{keys: left.keys, paths: paths}), true
		},
		applyScalar: func(_ coordinateKeyPayload, destination, fragment coordinateScalarPayload, affected bool) (coordinateScalarPayload, bool) {
			if affected {
				return fragment, true
			}
			return destination, true
		},
		affectedSelector: func(builder *boundaryAffectedSelectorBuilder, key coordinateKeyPayload) {
			builder.anyPaths(lenFloorCoordinateKeyValue(key).path)
		},
		applyRootSkeleton: func(_ *boundaryApplyContext, source coordinateSkeletonPayload, establishes bool) (coordinateSkeletonPayload, bool) {
			value := lenFloorCoordinateSkeletonValue(source)
			if establishes {
				value.bottom = false
			}
			return wrapLenFloorCoordinateSkeleton(value), true
		},
		rootSlot: func(_ *boundaryApplyContext, _ BoundaryFactorTarget) (coordinateKeyPayload, bool, bool) {
			return nil, false, true
		},
		rootScalar: func(_ *boundaryApplyContext, _ coordinateKeyPayload, _ product.Value) (coordinateScalarPayload, bool) {
			return nil, false
		},
	},
}

func buildLenFloorCoordinateFamily(_ *axis.Registry, _ DomainOptions) coordinateFamilyOps {
	elem := lenbound.ElementDomain()
	bottom := lenbound.BottomFloor()
	top := lenbound.TopFloor()
	return withSemanticSkeletonRepresentation(coordinateFamilyOps{
		branchRelation: coordinateIntegerBoundBranchRelation(
			CoordinateBoundLength, CoordinateBoundLower,
			func(keys *keyspace.KeySpace, path keyspace.Key) (coordinateKeyPayload, bool) {
				return wrapLenFloorCoordinateKey(path), lenFloorPathValid(path, keys)
			},
			applyLenFloorCoordinateBranchBound,
		),
		inventoryCompletion: noCoordinateInventoryCompletions(),
		requiredScalarKeys: func(skeleton coordinateSkeletonPayload) []coordinateKeyPayload {
			value := lenFloorCoordinateSkeletonValue(skeleton)
			if value.bottom {
				return nil
			}
			out := make([]coordinateKeyPayload, len(value.paths))
			for index, path := range value.paths {
				out[index] = wrapLenFloorCoordinateKey(path)
			}
			return out
		},
		sealSkeletonInventory: func(skeleton coordinateSkeletonPayload, admitted []coordinateKeyPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, bool) {
			value := lenFloorCoordinateSkeletonValue(skeleton)
			if value.keys != nil && value.keys != keys || keys == nil || !keys.Valid() {
				return nil, nil, false
			}
			if value.bottom {
				value.keys = keys
				return wrapLenFloorCoordinateSkeleton(value), nil, true
			}
			inventory := make(map[keyspace.Key]struct{}, len(admitted))
			for _, payload := range admitted {
				if payload == nil {
					return nil, nil, false
				}
				path := lenFloorCoordinateKeyValue(payload).path
				if !lenFloorPathValid(path, keys) {
					return nil, nil, false
				}
				inventory[path] = struct{}{}
			}
			paths := make([]keyspace.Key, 0, len(value.paths))
			for _, path := range value.paths {
				if _, ok := inventory[path]; ok {
					paths = append(paths, path)
				}
			}
			return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{keys: keys, paths: paths}), nil, true
		},
		sealSelectedSkeletonOverlay: func(selected []coordinateKeyPayload, _ *keyspace.KeySpace) (coordinateSkeletonOverlayPlanPayload, bool) {
			plan := make(lenFloorCoordinateOverlayPlan, len(selected))
			for index, payload := range selected {
				plan[index] = lenFloorCoordinateKeyValue(payload).path
			}
			return typedCoordinateSkeletonOverlayPlanPayload[lenFloorCoordinateOverlayPlan]{value: plan}, true
		},
		overlaySelectedSkeleton: func(payload coordinateSkeletonOverlayPlanPayload, current, image coordinateSkeletonPayload, _ []CoordinateScalarFactor, keys *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			left, right := lenFloorCoordinateSkeletonValue(current), lenFloorCoordinateSkeletonValue(image)
			typed, ok := payload.(typedCoordinateSkeletonOverlayPlanPayload[lenFloorCoordinateOverlayPlan])
			if !ok {
				return nil, false
			}
			selected := typed.value
			outPaths := overlaySelectedKeyspaceKeys(keys, left.paths, right.paths, selected)
			if len(selected) == 0 {
				left.keys = keys
				return wrapLenFloorCoordinateSkeleton(left), true
			}
			return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{
				bottom: len(outPaths) == 0 && right.bottom, keys: keys, paths: outPaths,
			}), true
		},
		decompose: func(payload laneFactorPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, error) {
			lane := typedLaneFactorValue[lenFloorLane](payload)
			if lane.lane.Bottom() {
				return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{bottom: true, keys: keys}), nil, nil
			}
			paths := make([]keyspace.Key, 0, len(lane.lane.Values()))
			for path, floor := range lane.lane.Values() {
				if !lenFloorPathValid(path, keys) || floor.Lo < 0 {
					return nil, nil, fmt.Errorf("invalid LenFloor coordinate")
				}
				paths = append(paths, path)
			}
			sort.Slice(paths, func(i, j int) bool { return keys.Less(paths[i], paths[j]) })
			entries := make([]coordinateEntry, len(paths))
			for index, path := range paths {
				entries[index] = coordinateEntry{key: wrapLenFloorCoordinateKey(path), scalar: wrapLenFloorCoordinateScalar(lane.lane.Values()[path])}
			}
			return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{keys: keys, paths: append([]keyspace.Key(nil), paths...)}), entries, nil
		},
		replace: func(_ laneFactorPayload, keys *keyspace.KeySpace, skeleton coordinateSkeletonPayload, entries []coordinateEntry) (laneFactorPayload, error) {
			topology := lenFloorCoordinateSkeletonValue(skeleton)
			if topology.bottom {
				if len(entries) != 0 {
					return nil, fmt.Errorf("LenFloor Bottom cannot carry coordinates")
				}
				return typedLaneFactorPayload[lenFloorLane]{value: lenFloorLane{lane: lift.MustMapBottom[keyspace.Key, lenbound.Floor]()}}, nil
			}
			if topology.keys != keys || len(topology.paths) != len(entries) {
				return nil, fmt.Errorf("LenFloor topology/scalar inventory mismatch")
			}
			values := make(map[keyspace.Key]lenbound.Floor, len(entries))
			for index, entry := range entries {
				path, floor := lenFloorCoordinateKeyValue(entry.key).path, lenFloorCoordinateScalarValue(entry.scalar).floor
				if !lenFloorPathValid(path, keys) || floor.Lo < 0 || path != topology.paths[index] {
					return nil, fmt.Errorf("invalid LenFloor coordinate %d", index)
				}
				if _, duplicate := values[path]; duplicate {
					return nil, fmt.Errorf("duplicate LenFloor coordinate")
				}
				values[path] = floor
			}
			return typedLaneFactorPayload[lenFloorLane]{value: lenFloorLane{lane: lift.MustMapValues(values)}}, nil
		},
		skeletonBottom: func() coordinateSkeletonPayload {
			return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{bottom: true})
		},
		skeletonTop: func() coordinateSkeletonPayload { return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{}) },
		skeletonEqual: func(a, b coordinateSkeletonPayload) bool {
			return equalLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(a), lenFloorCoordinateSkeletonValue(b))
		},
		skeletonLessOrEq: func(a, b coordinateSkeletonPayload) bool {
			left, right := lenFloorCoordinateSkeletonValue(a), lenFloorCoordinateSkeletonValue(b)
			if left.bottom {
				return true
			}
			if right.bottom {
				return false
			}
			return sortedLenFloorPathsSubset(right.paths, left.paths)
		},
		skeletonJoin: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapLenFloorCoordinateSkeleton(joinLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(a), lenFloorCoordinateSkeletonValue(b)))
		},
		skeletonMeet: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapLenFloorCoordinateSkeleton(meetLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(a), lenFloorCoordinateSkeletonValue(b)))
		},
		skeletonWiden: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapLenFloorCoordinateSkeleton(joinLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(a), lenFloorCoordinateSkeletonValue(b)))
		},
		skeletonNarrow: func(previous, _ coordinateSkeletonPayload) coordinateSkeletonPayload { return previous },
		skeletonHash: func(value coordinateSkeletonPayload) uint64 {
			v := lenFloorCoordinateSkeletonValue(value)
			hash := uint64(0)
			if v.bottom {
				return 1
			}
			for _, path := range v.paths {
				hash = internal.MixHash(hash, internal.FnvString(string(v.keys.FormatReadOnly(path))))
			}
			return hash
		},
		importSkeleton: func(source coordinateSkeletonPayload, from, to *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			value := lenFloorCoordinateSkeletonValue(source)
			if value.bottom {
				return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{bottom: true, keys: to}), true
			}
			paths := make([]keyspace.Key, len(value.paths))
			for i, path := range value.paths {
				next, ok := to.ImportKey(from, path)
				if !ok {
					return nil, false
				}
				paths[i] = next
			}
			sort.Slice(paths, func(i, j int) bool { return to.Less(paths[i], paths[j]) })
			return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{keys: to, paths: paths}), true
		},
		keyValid: func(key coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return lenFloorPathValid(lenFloorCoordinateKeyValue(key).path, keys)
		},
		keyEqual: func(a, b coordinateKeyPayload) bool {
			return lenFloorCoordinateKeyValue(a) == lenFloorCoordinateKeyValue(b)
		},
		keyLess: func(a, b coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return keys.Less(lenFloorCoordinateKeyValue(a).path, lenFloorCoordinateKeyValue(b).path)
		},
		keyHash: func(key coordinateKeyPayload, keys *keyspace.KeySpace) uint64 {
			return internal.FnvString(string(keys.FormatReadOnly(lenFloorCoordinateKeyValue(key).path)))
		},
		importKey: func(source coordinateKeyPayload, from, to *keyspace.KeySpace) (coordinateKeyPayload, bool) {
			path, ok := to.ImportKey(from, lenFloorCoordinateKeyValue(source).path)
			if !ok {
				return nil, false
			}
			return wrapLenFloorCoordinateKey(path), true
		},
		formalRekey: coordinateFormalRekeyPolicy{
			kind: coordinateFormalRekeyStructural,
			skeleton: func(source coordinateSkeletonPayload, plan CoordinateFormalRootRekey) (coordinateSkeletonPayload, bool) {
				value := lenFloorCoordinateSkeletonValue(source)
				if value.bottom {
					return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{bottom: true, keys: plan.to}), true
				}
				paths := make([]keyspace.Key, len(value.paths))
				for index, path := range value.paths {
					mapped, ok := plan.rekey(path)
					if !ok {
						return nil, false
					}
					paths[index] = mapped
				}
				sort.Slice(paths, func(i, j int) bool { return plan.to.Less(paths[i], paths[j]) })
				return wrapLenFloorCoordinateSkeleton(lenFloorCoordinateSkeleton{keys: plan.to, paths: paths}), true
			},
			key: func(source coordinateKeyPayload, plan CoordinateFormalRootRekey) (coordinateKeyPayload, bool) {
				path, ok := plan.rekey(lenFloorCoordinateKeyValue(source).path)
				if !ok {
					return nil, false
				}
				return wrapLenFloorCoordinateKey(path), true
			},
		},
		visitValueDependencies: func(coordinateKeyPayload, *keyspace.KeySpace, func(statekey.ValueDependency)) {},
		defaultScalar: func(skeleton coordinateSkeletonPayload, _ coordinateKeyPayload) (coordinateScalarPayload, error) {
			if lenFloorCoordinateSkeletonValue(skeleton).bottom {
				return wrapLenFloorCoordinateScalar(bottom), nil
			}
			return wrapLenFloorCoordinateScalar(top), nil
		},
		scalarSupport: func(skeleton coordinateSkeletonPayload, key coordinateKeyPayload) CoordinateScalarSupport {
			value := lenFloorCoordinateSkeletonValue(skeleton)
			if !value.bottom && sortedLenFloorPathContains(value.paths, lenFloorCoordinateKeyValue(key).path) {
				return CoordinateScalarRequired
			}
			return CoordinateScalarForbidden
		},
		scalarValid: func(_ coordinateKeyPayload, scalar coordinateScalarPayload) bool {
			floor := lenFloorCoordinateScalarValue(scalar).floor
			return floor.Lo >= 0
		},
		scalarEqual: func(a, b coordinateScalarPayload) bool {
			return elem.Equal(lenFloorCoordinateScalarValue(a).floor, lenFloorCoordinateScalarValue(b).floor)
		},
		scalarLessOrEq: func(a, b coordinateScalarPayload) bool {
			return elem.LessOrEq(lenFloorCoordinateScalarValue(a).floor, lenFloorCoordinateScalarValue(b).floor)
		},
		scalarJoin: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapLenFloorCoordinateScalar(elem.Join(lenFloorCoordinateScalarValue(a).floor, lenFloorCoordinateScalarValue(b).floor))
		},
		scalarMeet: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			left, right := lenFloorCoordinateScalarValue(a).floor, lenFloorCoordinateScalarValue(b).floor
			if left.Lo < right.Lo {
				left = right
			}
			return wrapLenFloorCoordinateScalar(left)
		},
		scalarWiden: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			left, right := lenFloorCoordinateScalarValue(a).floor, lenFloorCoordinateScalarValue(b).floor
			if elem.Equal(left, right) || elem.LessOrEq(right, left) {
				return wrapLenFloorCoordinateScalar(left)
			}
			return wrapLenFloorCoordinateScalar(elem.Widen(left, right))
		},
		scalarNarrow: func(previous, _ coordinateScalarPayload) coordinateScalarPayload { return previous },
		scalarHash: func(value coordinateScalarPayload) uint64 {
			return internal.MixHash(internal.FnvString("len-floor.coordinate"), uint64(lenFloorCoordinateScalarValue(value).floor.Lo))
		},
		importScalar: func(source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			floor := lenFloorCoordinateScalarValue(source).floor
			return wrapLenFloorCoordinateScalar(floor), floor.Lo >= 0
		},
		returnIdentity: noCoordinateReturnIdentity(), pathEvidence: noCoordinatePathEvidence(), pathValues: noCoordinatePathValues(), rootAssignment: lenFloorCoordinateRootAssignment(), pathMutation: lenFloorCoordinatePathMutation(), objectMutation: noCoordinateObjectMutation(),
	})
}

func overlaySelectedKeyspaceKeys(keys *keyspace.KeySpace, current, image, selected []keyspace.Key) []keyspace.Key {
	out := make([]keyspace.Key, 0, len(current)+len(selected))
	currentIndex, imageIndex, selectedIndex := 0, 0, 0
	imageHas := func(want keyspace.Key) bool {
		for imageIndex < len(image) && keys.Less(image[imageIndex], want) {
			imageIndex++
		}
		return imageIndex < len(image) && image[imageIndex] == want
	}
	for currentIndex < len(current) && selectedIndex < len(selected) {
		switch {
		case current[currentIndex] == selected[selectedIndex]:
			if imageHas(selected[selectedIndex]) {
				out = append(out, selected[selectedIndex])
			}
			currentIndex++
			selectedIndex++
		case keys.Less(current[currentIndex], selected[selectedIndex]):
			out = append(out, current[currentIndex])
			currentIndex++
		default:
			if imageHas(selected[selectedIndex]) {
				out = append(out, selected[selectedIndex])
			}
			selectedIndex++
		}
	}
	out = append(out, current[currentIndex:]...)
	for ; selectedIndex < len(selected); selectedIndex++ {
		if imageHas(selected[selectedIndex]) {
			out = append(out, selected[selectedIndex])
		}
	}
	return out
}

func lenFloorCoordinatePathMutation() coordinatePathMutationOps {
	affected := func(key coordinateKeyPayload, keys *keyspace.KeySpace, locations []CoordinateDependencyLocation) bool {
		path := lenFloorCoordinateKeyValue(key).path
		for _, location := range locations {
			if !location.IsRoot() && location.Path.Kind != keyspace.KindInvalid && keys.HasPrefix(path, location.Path) {
				return true
			}
		}
		return false
	}
	apply := func(skeleton coordinateSkeletonPayload, entries []coordinateEntry, remove func(keyspace.Key) bool) (coordinateSkeletonPayload, []coordinateEntry, bool) {
		topology := cloneLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(skeleton))
		if topology.bottom {
			return wrapLenFloorCoordinateSkeleton(topology), nil, true
		}
		paths := make([]keyspace.Key, 0, len(topology.paths))
		for _, path := range topology.paths {
			if !remove(path) {
				paths = append(paths, path)
			}
		}
		out := make([]coordinateEntry, 0, len(entries))
		for _, entry := range entries {
			if remove(lenFloorCoordinateKeyValue(entry.key).path) {
				continue
			}
			out = append(out, entry)
		}
		topology.paths = paths
		return wrapLenFloorCoordinateSkeleton(topology), out, true
	}
	return coordinatePathSubtreeMutation(
		affected,
		func(skeleton coordinateSkeletonPayload, entries []coordinateEntry, request pathSubtreeMutationRequest) (coordinateSkeletonPayload, []coordinateEntry, bool) {
			prefixes := prefixKeysOf(request.keys, request.prefixes)
			return apply(skeleton, entries, func(path keyspace.Key) bool {
				for _, prefix := range prefixes {
					if request.keys.HasPrefix(path, prefix) {
						return true
					}
				}
				return false
			})
		},
		func(skeleton coordinateSkeletonPayload, entries []coordinateEntry, request pathDescendantMutationRequest) (coordinateSkeletonPayload, []coordinateEntry, bool) {
			descendants := prefixKeysOf(request.keys, request.prefixes.Descendants)
			subtrees := prefixKeysOf(request.keys, request.prefixes.Subtrees)
			return apply(skeleton, entries, func(path keyspace.Key) bool {
				for _, prefix := range descendants {
					// A length floor depends on the complete child inventory.
					// Mutating any dynamic descendant invalidates the container's
					// own length fact as well as strict descendant facts.
					if request.keys.HasPrefix(path, prefix) {
						return true
					}
				}
				for _, prefix := range subtrees {
					if request.keys.HasPrefix(path, prefix) {
						return true
					}
				}
				return false
			})
		},
	)
}

func lenFloorCoordinateRootAssignment() coordinateRootAssignmentOps {
	ops := noCoordinateRootAssignment()
	ops.completionDependencies = rootAssignmentCompletionSourceValueDependencies()
	ops.completionTarget = func(target keyspace.Key) (coordinateKeyPayload, bool) {
		if target.Kind == keyspace.KindInvalid {
			return nil, false
		}
		return wrapLenFloorCoordinateKey(target), true
	}
	ops.completionSlot = func(completion RootAssignmentCompletion) (coordinateKeyPayload, bool) {
		if completion.lenFloor <= 0 || completion.lenFloorKey.Kind == keyspace.KindInvalid {
			return nil, false
		}
		return wrapLenFloorCoordinateKey(completion.lenFloorKey), true
	}
	ops.applyCompletion = func(skeleton coordinateSkeletonPayload, key coordinateKeyPayload, current coordinateScalarPayload, completion RootAssignmentCompletion) (coordinateSkeletonPayload, coordinateScalarPayload, bool) {
		path := lenFloorCoordinateKeyValue(key).path
		if completion.lenFloor <= 0 || path != completion.lenFloorKey {
			return nil, nil, false
		}
		topology := cloneLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(skeleton))
		floor := lenFloorCoordinateScalarValue(current).floor
		if topology.bottom {
			topology.bottom = false
			topology.paths = nil
			floor = lenbound.Floor{Lo: completion.lenFloor}
		} else if floor.Lo < completion.lenFloor {
			floor = lenbound.Floor{Lo: completion.lenFloor}
		}
		if !sortedLenFloorPathContains(topology.paths, path) {
			topology.paths = append(topology.paths, path)
			sort.Slice(topology.paths, func(i, j int) bool { return topology.keys.Less(topology.paths[i], topology.paths[j]) })
		}
		return wrapLenFloorCoordinateSkeleton(topology), wrapLenFloorCoordinateScalar(floor), true
	}
	ops.applyCompletionLane = func(payload laneFactorPayload, completion RootAssignmentCompletion) (laneFactorPayload, bool) {
		lane, changed := applyRootAssignmentLenFloor(typedLaneFactorValue[lenFloorLane](payload), completion)
		if !changed {
			return payload, false
		}
		return typedLaneFactorPayload[lenFloorLane]{value: lane}, true
	}
	ops.lenFloorValue = func(payload coordinateScalarPayload) (int64, bool) {
		floor := lenFloorCoordinateScalarValue(payload).floor.Lo
		return floor, floor > 0 && floor != lenbound.BottomFloor().Lo
	}
	return ops
}

func applyLenFloorCoordinateBranchBound(
	skeleton coordinateSkeletonPayload,
	key coordinateKeyPayload,
	current coordinateScalarPayload,
	bound int64,
) (coordinateSkeletonPayload, coordinateScalarPayload, bool) {
	path := lenFloorCoordinateKeyValue(key).path
	topology := cloneLenFloorCoordinateSkeleton(lenFloorCoordinateSkeletonValue(skeleton))
	if topology.keys == nil || !lenFloorPathValid(path, topology.keys) {
		return nil, nil, false
	}
	floor := lenFloorCoordinateScalarValue(current).floor
	if topology.bottom {
		topology.bottom = false
		topology.paths = nil
		floor = lenbound.TopFloor()
	}
	if bound > floor.Lo {
		floor = lenbound.Floor{Lo: bound}
	}
	if bound > 0 && !sortedLenFloorPathContains(topology.paths, path) {
		topology.paths = append(topology.paths, path)
		sort.Slice(topology.paths, func(i, j int) bool { return topology.keys.Less(topology.paths[i], topology.paths[j]) })
	}
	return wrapLenFloorCoordinateSkeleton(topology), wrapLenFloorCoordinateScalar(floor), true
}

func lenFloorPathValid(path keyspace.Key, keys *keyspace.KeySpace) bool {
	if keys == nil || !keys.Valid() || path.Kind == keyspace.KindInvalid {
		return false
	}
	_, ok := keys.SegmentsView(path)
	return ok
}
func wrapLenFloorCoordinateSkeleton(value lenFloorCoordinateSkeleton) coordinateSkeletonPayload {
	return typedCoordinateSkeletonPayload[lenFloorCoordinateSkeleton]{value: value}
}
func lenFloorCoordinateSkeletonValue(payload coordinateSkeletonPayload) lenFloorCoordinateSkeleton {
	typed, ok := payload.(typedCoordinateSkeletonPayload[lenFloorCoordinateSkeleton])
	if !ok {
		panic("state: LenFloor coordinate skeleton mismatch")
	}
	return typed.value
}
func wrapLenFloorCoordinateKey(path keyspace.Key) coordinateKeyPayload {
	return typedCoordinateKeyPayload[lenFloorCoordinateKey]{value: lenFloorCoordinateKey{path: path}}
}
func lenFloorCoordinateKeyValue(payload coordinateKeyPayload) lenFloorCoordinateKey {
	typed, ok := payload.(typedCoordinateKeyPayload[lenFloorCoordinateKey])
	if !ok {
		panic("state: LenFloor coordinate key mismatch")
	}
	return typed.value
}
func wrapLenFloorCoordinateScalar(floor lenbound.Floor) coordinateScalarPayload {
	return typedCoordinateScalarPayload[lenFloorCoordinateScalar]{value: lenFloorCoordinateScalar{floor: floor}}
}
func lenFloorCoordinateScalarValue(payload coordinateScalarPayload) lenFloorCoordinateScalar {
	typed, ok := payload.(typedCoordinateScalarPayload[lenFloorCoordinateScalar])
	if !ok {
		panic("state: LenFloor coordinate scalar mismatch")
	}
	return typed.value
}

func cloneLenFloorCoordinateSkeleton(value lenFloorCoordinateSkeleton) lenFloorCoordinateSkeleton {
	value.paths = append([]keyspace.Key(nil), value.paths...)
	return value
}

func sliceKeySet(paths []keyspace.Key) map[keyspace.Key]struct{} {
	out := make(map[keyspace.Key]struct{}, len(paths))
	for _, path := range paths {
		out[path] = struct{}{}
	}
	return out
}

func sortedLenFloorPathSet(keys *keyspace.KeySpace, values map[keyspace.Key]struct{}) []keyspace.Key {
	out := make([]keyspace.Key, 0, len(values))
	for path := range values {
		out = append(out, path)
	}
	sort.Slice(out, func(i, j int) bool { return keys.Less(out[i], out[j]) })
	return out
}

func filterLenFloorPaths(paths []keyspace.Key, keep func(keyspace.Key) bool) []keyspace.Key {
	out := make([]keyspace.Key, 0, len(paths))
	for _, path := range paths {
		if keep(path) {
			out = append(out, path)
		}
	}
	return out
}

func equalLenFloorCoordinateSkeleton(left, right lenFloorCoordinateSkeleton) bool {
	if left.bottom != right.bottom || len(left.paths) != len(right.paths) {
		return false
	}
	for index := range left.paths {
		if left.paths[index] != right.paths[index] {
			return false
		}
	}
	return true
}

func sortedLenFloorPathsSubset(subset, superset []keyspace.Key) bool {
	values := sliceKeySet(superset)
	for _, path := range subset {
		if _, ok := values[path]; !ok {
			return false
		}
	}
	return true
}

func sortedLenFloorPathContains(paths []keyspace.Key, candidate keyspace.Key) bool {
	for _, path := range paths {
		if path == candidate {
			return true
		}
	}
	return false
}

func joinLenFloorCoordinateSkeleton(left, right lenFloorCoordinateSkeleton) lenFloorCoordinateSkeleton {
	if left.bottom {
		return cloneLenFloorCoordinateSkeleton(right)
	}
	if right.bottom {
		return cloneLenFloorCoordinateSkeleton(left)
	}
	keys := left.keys
	if keys == nil {
		keys = right.keys
	}
	rightSet := sliceKeySet(right.paths)
	paths := make([]keyspace.Key, 0, min(len(left.paths), len(right.paths)))
	for _, path := range left.paths {
		if _, ok := rightSet[path]; ok {
			paths = append(paths, path)
		}
	}
	return lenFloorCoordinateSkeleton{keys: keys, paths: paths}
}

func meetLenFloorCoordinateSkeleton(left, right lenFloorCoordinateSkeleton) lenFloorCoordinateSkeleton {
	if left.bottom || right.bottom {
		keys := left.keys
		if keys == nil {
			keys = right.keys
		}
		return lenFloorCoordinateSkeleton{bottom: true, keys: keys}
	}
	keys := left.keys
	if keys == nil {
		keys = right.keys
	}
	values := sliceKeySet(left.paths)
	for _, path := range right.paths {
		values[path] = struct{}{}
	}
	return lenFloorCoordinateSkeleton{keys: keys, paths: sortedLenFloorPathSet(keys, values)}
}
