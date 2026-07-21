package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

const (
	numFloorCoordinateFamilyID CoordinateFamilyID = "numeric-lower-bound"
	numCeilCoordinateFamilyID  CoordinateFamilyID = "numeric-upper-bound"
)

type numBoundCoordinateSkeleton struct {
	bottom bool
	keys   *keyspace.KeySpace
}
type numBoundCoordinateKey struct{ path keyspace.Key }
type numBoundCoordinateOverlayPlan []numBoundCoordinateKey

// numBoundCoordinateScalar is one fixed cell of the finite must-map. Presence
// is semantic: an omitted key is map Top, while a present key whose element is
// itself Top is a strictly more precise fact and must not collapse with it.
type numBoundCoordinateScalar struct {
	value   int64
	present bool
}

func numBoundCoordinateFamilySpec(id CoordinateFamilyID, direction numbound.Direction, dynamicRead coordinateDynamicReadPolicy) coordinateFamilySpec {
	return coordinateFamilySpec{
		id: id, dynamicRead: dynamicRead, identityImage: IdentityImageIndependent,
		build: func(_ *axis.Registry, options DomainOptions) coordinateFamilyOps {
			return buildNumBoundCoordinateFamily(direction, options)
		},
		boundary: numBoundCoordinateBoundaryOps(),
	}
}

func numBoundCoordinateBoundaryOps() coordinateFamilyBoundaryOps {
	return coordinateFamilyBoundaryOps{
		admission: coordinateBoundaryAdmissionAllPreimages,
		rootUse:   boundaryRootUseReachability(),
		reachabilityKey: func(program *boundaryReachabilityProgramBuilder, source coordinateKeyPayload) {
			program.pathCone(false, numBoundCoordinateKeyValue(source).path)
		},
		projectSkeleton: func(ctx *boundaryProjectContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			value := numBoundCoordinateSkeletonValue(source)
			return wrapNumBoundCoordinateSkeleton(value), true
		},
		projectKey: func(ctx *boundaryProjectContext, source coordinateKeyPayload) (coordinateKeyPayload, bool, bool) {
			return source, ctx.closure.ContainsPath(numBoundCoordinateKeyValue(source).path), true
		},
		projectScalar: func(_ *boundaryProjectContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		rebaseSkeleton: func(ctx *boundaryRebaseContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			value := numBoundCoordinateSkeletonValue(source)
			if value.bottom {
				value.keys = ctx.toKeys
				return wrapNumBoundCoordinateSkeleton(value), true
			}
			value.keys = ctx.toKeys
			return wrapNumBoundCoordinateSkeleton(value), true
		},
		rebaseKeys: func(ctx *boundaryRebaseContext, source coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			paths, ok := boundaryRebasePaths(ctx, numBoundCoordinateKeyValue(source).path)
			if !ok {
				return nil, false
			}
			out := make([]coordinateKeyPayload, len(paths))
			for i, path := range paths {
				out[i] = wrapNumBoundCoordinateKey(path)
			}
			return out, true
		},
		rebaseScalar: func(_ *boundaryRebaseContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		sourceFiber: func(source coordinateKeyPayload) coordinateFiberPayload {
			return typedCoordinateFiberPayload[keyspace.Key]{value: numBoundCoordinateKeyValue(source).path}
		},
		inverseFibers: func(ctx *boundaryRebaseContext, destination coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
			paths, ok := ctx.quotient.pathPreimages(numBoundCoordinateKeyValue(destination).path)
			if !ok {
				return nil, false
			}
			out := make([]coordinateFiberPayload, len(paths))
			for i, path := range paths {
				out[i] = typedCoordinateFiberPayload[keyspace.Key]{value: path}
			}
			return out, true
		},
		postEntries: noCoordinatePostEntries,
		applySkeleton: func(ctx *boundaryApplyContext, destination, fragment coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			left, right := numBoundCoordinateSkeletonValue(destination), numBoundCoordinateSkeletonValue(fragment)
			if left.bottom || right.bottom {
				return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{bottom: true, keys: left.keys}), true
			}
			_ = ctx
			return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{keys: left.keys}), true
		},
		affectedSelector: func(builder *boundaryAffectedSelectorBuilder, key coordinateKeyPayload) {
			builder.anyPaths(numBoundCoordinateKeyValue(key).path)
		},
		applyScalar: func(_ coordinateKeyPayload, destination, fragment coordinateScalarPayload, affected bool) (coordinateScalarPayload, bool) {
			if affected {
				return fragment, true
			}
			return destination, true
		},
		applyRootSkeleton: func(_ *boundaryApplyContext, source coordinateSkeletonPayload, establishes bool) (coordinateSkeletonPayload, bool) {
			value := numBoundCoordinateSkeletonValue(source)
			if establishes {
				value.bottom = false
			}
			return wrapNumBoundCoordinateSkeleton(value), true
		},
		rootSlot: func(_ *boundaryApplyContext, _ BoundaryFactorTarget) (coordinateKeyPayload, bool, bool) {
			return nil, false, true
		},
		rootScalar: func(_ *boundaryApplyContext, _ coordinateKeyPayload, _ product.Value) (coordinateScalarPayload, bool) {
			return nil, false
		},
	}
}

func buildNumBoundCoordinateFamily(direction numbound.Direction, options DomainOptions) coordinateFamilyOps {
	thresholds := []int64(nil)
	if direction == numbound.Upper {
		thresholds = options.WidenThresholds
	}
	domain := numbound.IntDomain(numbound.Spec{Direction: direction, Bottom: numBoundBottom(direction), Top: numBoundTop(direction), Thresholds: thresholds})
	return withSemanticSkeletonRepresentation(coordinateFamilyOps{
		branchRelation: coordinateIntegerBoundBranchRelation(
			CoordinateBoundValue,
			func() CoordinateBoundDirection {
				if direction == numbound.Lower {
					return CoordinateBoundLower
				}
				return CoordinateBoundUpper
			}(),
			func(keys *keyspace.KeySpace, path keyspace.Key) (coordinateKeyPayload, bool) {
				return wrapNumBoundCoordinateKey(path), numBoundCoordinatePathValid(path, keys)
			},
			func(skeleton coordinateSkeletonPayload, key coordinateKeyPayload, current coordinateScalarPayload, bound int64) (coordinateSkeletonPayload, coordinateScalarPayload, bool) {
				return applyNumBoundCoordinateBranchBound(direction, skeleton, key, current, bound)
			},
		),
		inventoryCompletion: noCoordinateInventoryCompletions(),
		requiredScalarKeys:  func(coordinateSkeletonPayload) []coordinateKeyPayload { return nil },
		sealSkeletonInventory: func(skeleton coordinateSkeletonPayload, _ []coordinateKeyPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, bool) {
			value := numBoundCoordinateSkeletonValue(skeleton)
			if value.keys != nil && value.keys != keys || keys == nil || !keys.Valid() {
				return nil, nil, false
			}
			value.keys = keys
			return wrapNumBoundCoordinateSkeleton(value), nil, true
		},
		sealSelectedSkeletonOverlay: func(selected []coordinateKeyPayload, _ *keyspace.KeySpace) (coordinateSkeletonOverlayPlanPayload, bool) {
			plan := make(numBoundCoordinateOverlayPlan, len(selected))
			for index, payload := range selected {
				plan[index] = numBoundCoordinateKeyValue(payload)
			}
			return typedCoordinateSkeletonOverlayPlanPayload[numBoundCoordinateOverlayPlan]{value: plan}, true
		},
		overlaySelectedSkeleton: func(payload coordinateSkeletonOverlayPlanPayload, current, _ coordinateSkeletonPayload, _ []CoordinateScalarFactor, keys *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			_, ok := payload.(typedCoordinateSkeletonOverlayPlanPayload[numBoundCoordinateOverlayPlan])
			if !ok {
				return nil, false
			}
			left := numBoundCoordinateSkeletonValue(current)
			left.keys = keys
			return wrapNumBoundCoordinateSkeleton(left), true
		},
		decompose: func(payload laneFactorPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, error) {
			lane := typedLaneFactorValue[numBoundLane](payload)
			if lane.lane.Bottom() {
				return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{bottom: true, keys: keys}), nil, nil
			}
			paths := make([]keyspace.Key, 0, len(lane.lane.Values()))
			for path := range lane.lane.Values() {
				if !numBoundCoordinatePathValid(path, keys) {
					return nil, nil, fmt.Errorf("invalid numeric-bound coordinate")
				}
				paths = append(paths, path)
			}
			sort.Slice(paths, func(i, j int) bool { return keys.Less(paths[i], paths[j]) })
			entries := make([]coordinateEntry, len(paths))
			for i, path := range paths {
				entries[i] = coordinateEntry{key: wrapNumBoundCoordinateKey(path), scalar: wrapNumBoundCoordinateScalar(lane.lane.Values()[path])}
			}
			return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{keys: keys}), entries, nil
		},
		replace: func(_ laneFactorPayload, keys *keyspace.KeySpace, skeleton coordinateSkeletonPayload, entries []coordinateEntry) (laneFactorPayload, error) {
			topology := numBoundCoordinateSkeletonValue(skeleton)
			if topology.bottom {
				if len(entries) != 0 {
					return nil, fmt.Errorf("numeric-bound Bottom carries coordinates")
				}
				return typedLaneFactorPayload[numBoundLane]{value: numBoundLane{lane: lift.MustMapBottom[keyspace.Key, int64]()}}, nil
			}
			if topology.keys != keys {
				return nil, fmt.Errorf("numeric-bound keyspace mismatch")
			}
			values := make(map[keyspace.Key]int64, len(entries))
			for i, entry := range entries {
				path := numBoundCoordinateKeyValue(entry.key).path
				if !numBoundCoordinatePathValid(path, keys) || i != 0 && !keys.Less(numBoundCoordinateKeyValue(entries[i-1].key).path, path) {
					return nil, fmt.Errorf("invalid numeric-bound coordinate %d", i)
				}
				scalar := numBoundCoordinateScalarValue(entry.scalar)
				if scalar.present {
					values[path] = scalar.value
				}
			}
			return typedLaneFactorPayload[numBoundLane]{value: numBoundLane{lane: lift.MustMapValues(values)}}, nil
		},
		skeletonBottom: func() coordinateSkeletonPayload {
			return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{bottom: true})
		},
		skeletonTop: func() coordinateSkeletonPayload { return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{}) },
		skeletonEqual: func(a, b coordinateSkeletonPayload) bool {
			return numBoundCoordinateSkeletonValue(a).bottom == numBoundCoordinateSkeletonValue(b).bottom
		},
		skeletonLessOrEq: func(a, b coordinateSkeletonPayload) bool {
			left, right := numBoundCoordinateSkeletonValue(a), numBoundCoordinateSkeletonValue(b)
			if left.bottom {
				return true
			}
			return !right.bottom
		},
		skeletonJoin: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapNumBoundCoordinateSkeleton(joinNumBoundCoordinateSkeleton(numBoundCoordinateSkeletonValue(a), numBoundCoordinateSkeletonValue(b)))
		},
		skeletonMeet: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapNumBoundCoordinateSkeleton(meetNumBoundCoordinateSkeleton(numBoundCoordinateSkeletonValue(a), numBoundCoordinateSkeletonValue(b)))
		},
		skeletonWiden: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapNumBoundCoordinateSkeleton(joinNumBoundCoordinateSkeleton(numBoundCoordinateSkeletonValue(a), numBoundCoordinateSkeletonValue(b)))
		},
		skeletonNarrow: func(previous, _ coordinateSkeletonPayload) coordinateSkeletonPayload { return previous },
		skeletonHash: func(payload coordinateSkeletonPayload) uint64 {
			if numBoundCoordinateSkeletonValue(payload).bottom {
				return 1
			}
			return 0
		},
		importSkeleton: func(source coordinateSkeletonPayload, from, to *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			value := numBoundCoordinateSkeletonValue(source)
			if value.bottom {
				return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{bottom: true, keys: to}), true
			}
			_ = from
			return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{keys: to}), true
		},
		keyValid: func(key coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return numBoundCoordinatePathValid(numBoundCoordinateKeyValue(key).path, keys)
		},
		keyEqual: func(a, b coordinateKeyPayload) bool {
			return numBoundCoordinateKeyValue(a) == numBoundCoordinateKeyValue(b)
		},
		keyLess: func(a, b coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return keys.Less(numBoundCoordinateKeyValue(a).path, numBoundCoordinateKeyValue(b).path)
		},
		keyHash: func(key coordinateKeyPayload, keys *keyspace.KeySpace) uint64 {
			return internal.FnvString(string(keys.FormatReadOnly(numBoundCoordinateKeyValue(key).path)))
		},
		importKey: func(source coordinateKeyPayload, from, to *keyspace.KeySpace) (coordinateKeyPayload, bool) {
			path, ok := to.ImportKey(from, numBoundCoordinateKeyValue(source).path)
			if !ok {
				return nil, false
			}
			return wrapNumBoundCoordinateKey(path), true
		},
		formalRekey: coordinateFormalRekeyPolicy{
			kind: coordinateFormalRekeyStructural,
			skeleton: func(source coordinateSkeletonPayload, plan CoordinateFormalRootRekey) (coordinateSkeletonPayload, bool) {
				value := numBoundCoordinateSkeletonValue(source)
				return wrapNumBoundCoordinateSkeleton(numBoundCoordinateSkeleton{bottom: value.bottom, keys: plan.to}), true
			},
			key: func(source coordinateKeyPayload, plan CoordinateFormalRootRekey) (coordinateKeyPayload, bool) {
				path, ok := plan.rekey(numBoundCoordinateKeyValue(source).path)
				if !ok {
					return nil, false
				}
				return wrapNumBoundCoordinateKey(path), true
			},
		},
		visitValueDependencies: func(coordinateKeyPayload, *keyspace.KeySpace, func(statekey.ValueDependency)) {},
		defaultScalar: func(skeleton coordinateSkeletonPayload, _ coordinateKeyPayload) (coordinateScalarPayload, error) {
			if numBoundCoordinateSkeletonValue(skeleton).bottom {
				return wrapNumBoundCoordinateScalar(numBoundBottom(direction)), nil
			}
			return wrapOmittedNumBoundCoordinateScalar(direction), nil
		},
		scalarSupport: func(skeleton coordinateSkeletonPayload, _ coordinateKeyPayload) CoordinateScalarSupport {
			if !numBoundCoordinateSkeletonValue(skeleton).bottom {
				return CoordinateScalarOptional
			}
			return CoordinateScalarForbidden
		},
		scalarValid: func(_ coordinateKeyPayload, value coordinateScalarPayload) bool {
			scalar := numBoundCoordinateScalarValue(value)
			return scalar.present || scalar.value == numBoundTop(direction)
		},
		scalarEqual: func(a, b coordinateScalarPayload) bool {
			left, right := numBoundCoordinateScalarValue(a), numBoundCoordinateScalarValue(b)
			return left.present == right.present && (!left.present || domain.Equal(left.value, right.value))
		},
		scalarLessOrEq: func(a, b coordinateScalarPayload) bool {
			left, right := numBoundCoordinateScalarValue(a), numBoundCoordinateScalarValue(b)
			if !right.present {
				return true
			}
			return left.present && domain.LessOrEq(left.value, right.value)
		},
		scalarJoin: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			left, right := numBoundCoordinateScalarValue(a), numBoundCoordinateScalarValue(b)
			if !left.present || !right.present {
				return wrapOmittedNumBoundCoordinateScalar(direction)
			}
			return wrapNumBoundCoordinateScalar(domain.Join(left.value, right.value))
		},
		scalarMeet: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			left, right := numBoundCoordinateScalarValue(a), numBoundCoordinateScalarValue(b)
			if !left.present {
				return b
			}
			if !right.present {
				return a
			}
			value := left.value
			if direction == numbound.Lower && right.value > value || direction == numbound.Upper && right.value < value {
				value = right.value
			}
			return wrapNumBoundCoordinateScalar(value)
		},
		scalarWiden: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			left, right := numBoundCoordinateScalarValue(a), numBoundCoordinateScalarValue(b)
			if !left.present {
				return b
			}
			if !right.present {
				return wrapOmittedNumBoundCoordinateScalar(direction)
			}
			return wrapNumBoundCoordinateScalar(domain.Widen(left.value, right.value))
		},
		scalarNarrow: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			if domain.Narrow == nil {
				return a
			}
			left, right := numBoundCoordinateScalarValue(a), numBoundCoordinateScalarValue(b)
			leftValue, rightValue := left.value, right.value
			if !left.present {
				leftValue = numBoundTop(direction)
			}
			if !right.present {
				rightValue = numBoundTop(direction)
			}
			narrowed := domain.Narrow(leftValue, rightValue)
			if domain.Equal(narrowed, numBoundTop(direction)) {
				return wrapOmittedNumBoundCoordinateScalar(direction)
			}
			return wrapNumBoundCoordinateScalar(narrowed)
		},
		scalarHash: func(value coordinateScalarPayload) uint64 {
			scalar := numBoundCoordinateScalarValue(value)
			if !scalar.present {
				return internal.MixHash(uint64(direction), 0)
			}
			return internal.MixHash(internal.MixHash(uint64(direction), 1), uint64(scalar.value))
		},
		importScalar: func(source coordinateScalarPayload) (coordinateScalarPayload, bool) { return source, true },
		reachableDefault: func(coordinateKeyPayload) (coordinateScalarPayload, bool) {
			return wrapOmittedNumBoundCoordinateScalar(direction), true
		},
		returnIdentity: noCoordinateReturnIdentity(), pathEvidence: noCoordinatePathEvidence(), pathValues: noCoordinatePathValues(), rootAssignment: numBoundCoordinateRootAssignment(direction), pathMutation: noCoordinatePathMutation(), objectMutation: noCoordinateObjectMutation(),
	})
}

func applyNumBoundCoordinateBranchBound(
	direction numbound.Direction,
	skeleton coordinateSkeletonPayload,
	key coordinateKeyPayload,
	current coordinateScalarPayload,
	bound int64,
) (coordinateSkeletonPayload, coordinateScalarPayload, bool) {
	path := numBoundCoordinateKeyValue(key).path
	topology := numBoundCoordinateSkeletonValue(skeleton)
	if topology.keys == nil || !numBoundCoordinatePathValid(path, topology.keys) {
		return nil, nil, false
	}
	value := numBoundCoordinateScalarValue(current)
	if topology.bottom {
		topology.bottom = false
		value = numBoundCoordinateScalar{value: numBoundTop(direction)}
	}
	if !value.present || !numBoundStrongerOrEqual(direction, value.value, bound) {
		value = numBoundCoordinateScalar{value: bound, present: true}
	}
	return wrapNumBoundCoordinateSkeleton(topology), typedCoordinateScalarPayload[numBoundCoordinateScalar]{value: value}, true
}

func numBoundCoordinateRootAssignment(direction numbound.Direction) coordinateRootAssignmentOps {
	ops := noCoordinateRootAssignment()
	ops.scalarTransfer = coordinateScalarTransferOps{
		kind: coordinateScalarTransferParticipant,
		demand: func(_ []coordinateKeyPayload, transfer RootAssignmentScalarTransfer) ([]coordinateScalarTransferDemand, bool) {
			bound := rootAssignmentNumBoundForDirection(transfer, direction)
			demand := coordinateScalarTransferDemand{target: wrapNumBoundCoordinateKey(transfer.target)}
			if bound.present && !bound.exact {
				demand.source, demand.hasSource = wrapNumBoundCoordinateKey(bound.source), true
			}
			return []coordinateScalarTransferDemand{demand}, true
		},
		apply: func(currentSkeleton coordinateSkeletonPayload, _ coordinateScalarPayload, pointSource coordinateScalarPayload, hasSource bool, transfer RootAssignmentScalarTransfer) (coordinateSkeletonPayload, coordinateScalarPayload, bool) {
			topology := numBoundCoordinateSkeletonValue(currentSkeleton)
			bound := rootAssignmentNumBoundForDirection(transfer, direction)
			sourceValue, sourcePresent := int64(0), false
			if hasSource {
				source := numBoundCoordinateScalarValue(pointSource)
				sourceValue, sourcePresent = source.value, source.present
			}
			value, present := resolveRootAssignmentNumBound(bound, sourceValue, sourcePresent)
			if topology.bottom && !present {
				return wrapNumBoundCoordinateSkeleton(topology), wrapNumBoundCoordinateScalar(numBoundBottom(direction)), true
			}
			topology.bottom = false
			if present {
				return wrapNumBoundCoordinateSkeleton(topology), wrapNumBoundCoordinateScalar(value), true
			}
			return wrapNumBoundCoordinateSkeleton(topology), wrapOmittedNumBoundCoordinateScalar(direction), true
		},
	}
	return ops
}

func rootAssignmentNumBoundForDirection(transfer RootAssignmentScalarTransfer, direction numbound.Direction) RootAssignmentNumBound {
	if direction == numbound.Lower {
		return transfer.numFloor
	}
	return transfer.numCeil
}
func resolveRootAssignmentNumBound(bound RootAssignmentNumBound, source int64, sourcePresent bool) (int64, bool) {
	if !bound.present {
		return 0, false
	}
	if bound.exact {
		return bound.value, true
	}
	if !sourcePresent {
		return 0, false
	}
	return addRootAssignmentInt64(source, bound.value)
}
func numBoundBottom(direction numbound.Direction) int64 {
	if direction == numbound.Lower {
		return maxNumBound
	}
	return minNumBound
}
func numBoundTop(direction numbound.Direction) int64 {
	if direction == numbound.Lower {
		return minNumBound
	}
	return maxNumBound
}
func wrapNumBoundCoordinateSkeleton(value numBoundCoordinateSkeleton) coordinateSkeletonPayload {
	return typedCoordinateSkeletonPayload[numBoundCoordinateSkeleton]{value: value}
}
func numBoundCoordinateSkeletonValue(payload coordinateSkeletonPayload) numBoundCoordinateSkeleton {
	typed, ok := payload.(typedCoordinateSkeletonPayload[numBoundCoordinateSkeleton])
	if !ok {
		panic("state: numeric-bound coordinate skeleton payload mismatch")
	}
	return typed.value
}
func wrapNumBoundCoordinateKey(path keyspace.Key) coordinateKeyPayload {
	return typedCoordinateKeyPayload[numBoundCoordinateKey]{value: numBoundCoordinateKey{path: path}}
}
func numBoundCoordinateKeyValue(payload coordinateKeyPayload) numBoundCoordinateKey {
	typed, ok := payload.(typedCoordinateKeyPayload[numBoundCoordinateKey])
	if !ok {
		panic("state: numeric-bound coordinate key payload mismatch")
	}
	return typed.value
}
func wrapNumBoundCoordinateScalar(value int64) coordinateScalarPayload {
	return typedCoordinateScalarPayload[numBoundCoordinateScalar]{value: numBoundCoordinateScalar{value: value, present: true}}
}
func wrapOmittedNumBoundCoordinateScalar(direction numbound.Direction) coordinateScalarPayload {
	return typedCoordinateScalarPayload[numBoundCoordinateScalar]{value: numBoundCoordinateScalar{value: numBoundTop(direction)}}
}
func numBoundCoordinateScalarValue(payload coordinateScalarPayload) numBoundCoordinateScalar {
	typed, ok := payload.(typedCoordinateScalarPayload[numBoundCoordinateScalar])
	if !ok {
		panic("state: numeric-bound coordinate scalar payload mismatch")
	}
	return typed.value
}
func numBoundCoordinatePathValid(path keyspace.Key, keys *keyspace.KeySpace) bool {
	if keys == nil || !keys.Valid() || path.Kind == keyspace.KindInvalid {
		return false
	}
	_, ok := keys.SegmentsView(path)
	return ok
}
func joinNumBoundCoordinateSkeleton(a, b numBoundCoordinateSkeleton) numBoundCoordinateSkeleton {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return numBoundCoordinateSkeleton{keys: a.keys}
}
func meetNumBoundCoordinateSkeleton(a, b numBoundCoordinateSkeleton) numBoundCoordinateSkeleton {
	if a.bottom || b.bottom {
		return numBoundCoordinateSkeleton{bottom: true, keys: a.keys}
	}
	return numBoundCoordinateSkeleton{keys: a.keys}
}
