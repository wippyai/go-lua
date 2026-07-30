package state

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

// A relation coordinate is one normalized relation shape. The scalar contains
// only the exact finite must-set of K bounds for that shape. Consequently an
// exact RelConstraint has one owner, while the skeleton's immutable incidence
// index can plan a finite connected component without observing any scalar.
const diffRelationCoordinateFamilyID CoordinateFamilyID = "relation-shape"

type diffRelationCoordinateOperand struct {
	path  keyspace.Key
	kind  RelOperandKind
	state pathaddr.StateKey
}

type diffRelationCoordinateKey struct {
	coA int64
	a   diffRelationCoordinateOperand
	coB int64
	b   diffRelationCoordinateOperand
	c   diffRelationCoordinateOperand
}

type diffRelationCoordinateSkeleton struct {
	bottom bool
	keys   *keyspace.KeySpace
	// shapes is sorted and duplicate-free. It is structural query-planning
	// inventory, never a second store of scalar relation facts.
	shapes []diffRelationCoordinateKey
	// incidence is the sealed transpose used by query planning. Indices point
	// into shapes and are strictly increasing.
	incidence map[diffRelationCoordinateOperand][]int
}

type diffRelationCoordinateScalar struct {
	bottom bool
	bounds []int64
}
type diffRelationCoordinateOverlayPlan []diffRelationCoordinateKey

var diffRelationCoordinateFamilySpec = coordinateFamilySpec{
	id: diffRelationCoordinateFamilyID, dynamicRead: dynamicReadDiffRelationCoordinates(),
	identityImage: IdentityImageIndependent,
	build:         buildDiffRelationCoordinateFamily, boundary: diffRelationCoordinateBoundaryOps(),
}

func buildDiffRelationCoordinateFamily(_ *axis.Registry, _ DomainOptions) coordinateFamilyOps {
	ops := coordinateFamilyOps{
		branchRelation: coordinateLinearConstraintBranchRelation(
			func(keys *keyspace.KeySpace, constraint RelConstraint) (coordinateKeyPayload, bool) {
				canonical, ok := canonicalRelConstraint(constraint)
				if !ok {
					return nil, false
				}
				key, ok := diffRelationCoordinateKeyFromConstraint(keys, canonical)
				return wrapDiffRelationCoordinateKey(key), ok
			},
			applyDiffRelationCoordinateBranchConstraint,
		),
		inventoryCompletion: noCoordinateInventoryCompletions(),
		requiredScalarKeys:  func(coordinateSkeletonPayload) []coordinateKeyPayload { return nil },
		sealSkeletonInventory: func(skeleton coordinateSkeletonPayload, admitted []coordinateKeyPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, bool) {
			value := diffRelationCoordinateSkeletonValue(skeleton)
			if value.keys != nil && value.keys != keys || keys == nil || !keys.Valid() {
				return nil, nil, false
			}
			if value.bottom {
				value.keys = keys
				return wrapDiffRelationCoordinateSkeleton(value), nil, true
			}
			// Join may transiently union shapes to align scalar intersection.
			// Retained relation topology is exactly the support of the surviving
			// non-default must scalars, so prune the alignment inventory here.
			kept := make([]diffRelationCoordinateKey, 0, len(admitted))
			for _, payload := range admitted {
				if payload == nil {
					return nil, nil, false
				}
				shape := diffRelationCoordinateKeyValue(payload)
				if !diffRelationCoordinateKeyValid(shape, keys) {
					return nil, nil, false
				}
				if diffRelationShapePresent(value.shapes, shape, keys) {
					kept = append(kept, shape)
				}
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(keys, false, kept)), nil, true
		},
		sealSelectedSkeletonOverlay: func(selected []coordinateKeyPayload, _ *keyspace.KeySpace) (coordinateSkeletonOverlayPlanPayload, bool) {
			plan := make(diffRelationCoordinateOverlayPlan, len(selected))
			for index, payload := range selected {
				plan[index] = diffRelationCoordinateKeyValue(payload)
			}
			return typedCoordinateSkeletonOverlayPlanPayload[diffRelationCoordinateOverlayPlan]{value: plan}, true
		},
		overlaySelectedSkeleton: func(payload coordinateSkeletonOverlayPlanPayload, current, image coordinateSkeletonPayload, _ []CoordinateScalarFactor, keys *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			left, right := diffRelationCoordinateSkeletonValue(current), diffRelationCoordinateSkeletonValue(image)
			typed, ok := payload.(typedCoordinateSkeletonOverlayPlanPayload[diffRelationCoordinateOverlayPlan])
			if !ok {
				return nil, false
			}
			selected := typed.value
			shapes := overlayDiffRelationCoordinateShapes(keys, left.shapes, right.shapes, selected)
			if len(selected) == 0 {
				return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(keys, left.bottom, left.shapes)), true
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(keys, left.bottom, shapes)), true
		},
		decompose: func(payload laneFactorPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, error) {
			lane := typedLaneFactorValue[diffRelationLane](payload)
			if lane.bottom {
				return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{bottom: true, keys: keys}), nil, nil
			}
			if keys == nil || !keys.Valid() {
				return nil, nil, fmt.Errorf("invalid relation coordinate keyspace")
			}
			grouped := make(map[diffRelationCoordinateKey][]int64)
			for relation := range lane.values {
				relation, valid := canonicalRelConstraint(relation)
				key, keyOK := diffRelationCoordinateKeyFromConstraint(keys, relation)
				if !valid || !keyOK {
					return nil, nil, fmt.Errorf("invalid relation constraint")
				}
				grouped[key] = append(grouped[key], relation.K)
			}
			shapes := make([]diffRelationCoordinateKey, 0, len(grouped))
			for key := range grouped {
				shapes = append(shapes, key)
			}
			sort.Slice(shapes, func(i, j int) bool { return diffRelationCoordinateKeyLess(shapes[i], shapes[j], keys) })
			entries := make([]coordinateEntry, len(shapes))
			for i, key := range shapes {
				entries[i] = coordinateEntry{key: wrapDiffRelationCoordinateKey(key), scalar: wrapDiffRelationCoordinateScalar(diffRelationCoordinateScalar{bounds: canonicalInt64Set(grouped[key])})}
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(keys, false, shapes)), entries, nil
		},
		replace: func(_ laneFactorPayload, keys *keyspace.KeySpace, skeleton coordinateSkeletonPayload, entries []coordinateEntry) (laneFactorPayload, error) {
			topology := diffRelationCoordinateSkeletonValue(skeleton)
			if topology.bottom {
				if len(entries) != 0 {
					return nil, fmt.Errorf("relation Bottom carries coordinates")
				}
				return typedLaneFactorPayload[diffRelationLane]{value: diffRelationLane{mustSetLane: mustSetLane[RelConstraint]{bottom: true}}}, nil
			}
			if topology.keys != keys || keys == nil || !keys.Valid() || !diffRelationShapesCanonical(topology.shapes, keys) || !diffRelationIncidenceValid(topology) {
				return nil, fmt.Errorf("relation coordinate skeleton mismatch")
			}
			values := make(map[RelConstraint]struct{})
			for i, entry := range entries {
				key := diffRelationCoordinateKeyValue(entry.key)
				if !diffRelationCoordinateKeyValid(key, keys) || !diffRelationShapePresent(topology.shapes, key, keys) || i > 0 && !diffRelationCoordinateKeyLess(diffRelationCoordinateKeyValue(entries[i-1].key), key, keys) {
					return nil, fmt.Errorf("invalid relation coordinate %d", i)
				}
				scalar := diffRelationCoordinateScalarValue(entry.scalar)
				if scalar.bottom || !int64SetCanonical(scalar.bounds) {
					return nil, fmt.Errorf("invalid relation coordinate scalar")
				}
				for _, bound := range scalar.bounds {
					relation, ok := diffRelationConstraintFromCoordinate(keys, key, bound)
					if !ok {
						return nil, fmt.Errorf("invalid relation shape")
					}
					values[relation] = struct{}{}
				}
			}
			return typedLaneFactorPayload[diffRelationLane]{value: diffRelationLane{mustSetLane: mustSetLane[RelConstraint]{values: values}}}, nil
		},
		skeletonBottom: func() coordinateSkeletonPayload {
			return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{bottom: true})
		},
		skeletonTop: func() coordinateSkeletonPayload {
			return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{})
		},
		skeletonEqual: func(a, b coordinateSkeletonPayload) bool {
			left, right := diffRelationCoordinateSkeletonValue(a), diffRelationCoordinateSkeletonValue(b)
			return left.bottom == right.bottom
		},
		skeletonLessOrEq: func(a, b coordinateSkeletonPayload) bool {
			left, right := diffRelationCoordinateSkeletonValue(a), diffRelationCoordinateSkeletonValue(b)
			return left.bottom || !right.bottom
		},
		skeletonJoin: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			left, right := diffRelationCoordinateSkeletonValue(a), diffRelationCoordinateSkeletonValue(b)
			if left.bottom {
				return b
			}
			if right.bottom {
				return a
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(left.keys, false, mergeDiffRelationShapes(left.shapes, right.shapes, left.keys, false)))
		},
		skeletonMeet: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			left, right := diffRelationCoordinateSkeletonValue(a), diffRelationCoordinateSkeletonValue(b)
			if left.bottom || right.bottom {
				return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{bottom: true, keys: left.keys})
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(left.keys, false, mergeDiffRelationShapes(left.shapes, right.shapes, left.keys, false)))
		},
		skeletonWiden: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			left, right := diffRelationCoordinateSkeletonValue(a), diffRelationCoordinateSkeletonValue(b)
			if left.bottom {
				return b
			}
			if right.bottom {
				return a
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(left.keys, false, mergeDiffRelationShapes(left.shapes, right.shapes, left.keys, false)))
		},
		skeletonNarrow: func(previous, next coordinateSkeletonPayload) coordinateSkeletonPayload {
			left, right := diffRelationCoordinateSkeletonValue(previous), diffRelationCoordinateSkeletonValue(next)
			if left.bottom || right.bottom {
				return previous
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(left.keys, false, mergeDiffRelationShapes(left.shapes, right.shapes, left.keys, false)))
		},
		skeletonHash: func(payload coordinateSkeletonPayload) uint64 {
			value := diffRelationCoordinateSkeletonValue(payload)
			if value.bottom {
				return 1
			}
			return 0
		},
		importSkeleton: func(source coordinateSkeletonPayload, from, to *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			value := diffRelationCoordinateSkeletonValue(source)
			if value.bottom {
				return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{bottom: true, keys: to}), true
			}
			out := make([]diffRelationCoordinateKey, 0, len(value.shapes))
			for _, shape := range value.shapes {
				mapped, ok := importDiffRelationCoordinateKey(shape, from, to)
				if !ok {
					return nil, false
				}
				out = append(out, mapped)
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(to, false, out)), true
		},
		keyValid: func(key coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return diffRelationCoordinateKeyValid(diffRelationCoordinateKeyValue(key), keys)
		},
		keyEqual: func(a, b coordinateKeyPayload) bool {
			return diffRelationCoordinateKeyValue(a) == diffRelationCoordinateKeyValue(b)
		},
		keyLess: func(a, b coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return diffRelationCoordinateKeyLess(diffRelationCoordinateKeyValue(a), diffRelationCoordinateKeyValue(b), keys)
		},
		keyHash: func(payload coordinateKeyPayload, keys *keyspace.KeySpace) uint64 {
			return hashDiffRelationCoordinateKey(diffRelationCoordinateKeyValue(payload), keys)
		},
		importKey: func(source coordinateKeyPayload, from, to *keyspace.KeySpace) (coordinateKeyPayload, bool) {
			key, ok := importDiffRelationCoordinateKey(diffRelationCoordinateKeyValue(source), from, to)
			if !ok {
				return nil, false
			}
			return wrapDiffRelationCoordinateKey(key), true
		},
		formalRekey: coordinateFormalRekeyPolicy{
			kind: coordinateFormalRekeyStructural,
			skeleton: func(source coordinateSkeletonPayload, plan CoordinateFormalRootRekey) (coordinateSkeletonPayload, bool) {
				value := diffRelationCoordinateSkeletonValue(source)
				if value.bottom {
					return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{bottom: true, keys: plan.to}), true
				}
				shapes := make([]diffRelationCoordinateKey, len(value.shapes))
				for index, shape := range value.shapes {
					mapped, ok := mapDiffRelationCoordinateKey(shape, plan.to, plan.rekey)
					if !ok {
						return nil, false
					}
					shapes[index] = mapped
				}
				return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(plan.to, false, shapes)), true
			},
			key: func(source coordinateKeyPayload, plan CoordinateFormalRootRekey) (coordinateKeyPayload, bool) {
				key, ok := mapDiffRelationCoordinateKey(diffRelationCoordinateKeyValue(source), plan.to, plan.rekey)
				if !ok {
					return nil, false
				}
				return wrapDiffRelationCoordinateKey(key), true
			},
		},
		visitValueDependencies: func(coordinateKeyPayload, *keyspace.KeySpace, func(statekey.ValueDependency)) {},
		defaultScalar: func(skeleton coordinateSkeletonPayload, _ coordinateKeyPayload) (coordinateScalarPayload, error) {
			if diffRelationCoordinateSkeletonValue(skeleton).bottom {
				return wrapDiffRelationCoordinateScalar(diffRelationCoordinateScalar{bottom: true}), nil
			}
			return wrapDiffRelationCoordinateScalar(diffRelationCoordinateScalar{}), nil
		},
		scalarSupport: func(skeleton coordinateSkeletonPayload, key coordinateKeyPayload) CoordinateScalarSupport {
			value := diffRelationCoordinateSkeletonValue(skeleton)
			if !value.bottom && diffRelationShapePresent(value.shapes, diffRelationCoordinateKeyValue(key), value.keys) {
				return CoordinateScalarOptional
			}
			return CoordinateScalarForbidden
		},
		scalarValid: func(_ coordinateKeyPayload, payload coordinateScalarPayload) bool {
			value := diffRelationCoordinateScalarValue(payload)
			return value.bottom && len(value.bounds) == 0 || !value.bottom && int64SetCanonical(value.bounds)
		},
		scalarEqual: func(a, b coordinateScalarPayload) bool {
			return equalDiffRelationCoordinateScalar(diffRelationCoordinateScalarValue(a), diffRelationCoordinateScalarValue(b))
		},
		scalarLessOrEq: func(a, b coordinateScalarPayload) bool {
			return lessOrEqDiffRelationCoordinateScalar(diffRelationCoordinateScalarValue(a), diffRelationCoordinateScalarValue(b))
		},
		scalarJoin: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapDiffRelationCoordinateScalar(joinDiffRelationCoordinateScalar(diffRelationCoordinateScalarValue(a), diffRelationCoordinateScalarValue(b)))
		},
		scalarMeet: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapDiffRelationCoordinateScalar(meetDiffRelationCoordinateScalar(diffRelationCoordinateScalarValue(a), diffRelationCoordinateScalarValue(b)))
		},
		scalarWiden: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapDiffRelationCoordinateScalar(joinDiffRelationCoordinateScalar(diffRelationCoordinateScalarValue(a), diffRelationCoordinateScalarValue(b)))
		},
		scalarNarrow: func(previous, _ coordinateScalarPayload) coordinateScalarPayload { return previous },
		scalarHash: func(payload coordinateScalarPayload) uint64 {
			value := diffRelationCoordinateScalarValue(payload)
			if value.bottom {
				return 1
			}
			h := uint64(0)
			for _, bound := range value.bounds {
				h = internal.MixHash(h, uint64(bound))
			}
			return h
		},
		importScalar: func(source coordinateScalarPayload) (coordinateScalarPayload, bool) { return source, true },
		reachableDefault: func(coordinateKeyPayload) (coordinateScalarPayload, bool) {
			return wrapDiffRelationCoordinateScalar(diffRelationCoordinateScalar{}), true
		},
		returnIdentity: noCoordinateReturnIdentity(), pathEvidence: noCoordinatePathEvidence(), pathValues: noCoordinatePathValues(), rootAssignment: diffRelationCoordinateRootAssignment(), pathMutation: noCoordinatePathMutation(), objectMutation: noCoordinateObjectMutation(),
	}
	// Reachable relation skeletons are semantically equal because topology is
	// only the transient alignment inventory for must-set scalar intersection.
	// They are not representation-substitutable: scalarSupport is defined by
	// the exact surviving shape inventory. Retained factors are decomposed from
	// the canonical relation lane, so this identity is the pruned normal form
	// and prevents one shape inventory from admitting/omitting another's scalar.
	ops.skeletonRepresentationEqual = func(a, b coordinateSkeletonPayload) bool {
		left, right := diffRelationCoordinateSkeletonValue(a), diffRelationCoordinateSkeletonValue(b)
		if left.bottom != right.bottom {
			return false
		}
		return left.bottom || left.keys == right.keys && diffRelationShapesEqual(left.shapes, right.shapes)
	}
	ops.skeletonRepresentationHash = func(payload coordinateSkeletonPayload) uint64 {
		value := diffRelationCoordinateSkeletonValue(payload)
		if value.bottom {
			return 1
		}
		h := uint64(1469598103934665603)
		for _, shape := range value.shapes {
			h = internal.MixHash(h, hashDiffRelationCoordinateKey(shape, value.keys))
		}
		return h
	}
	// Scalar spellings are already canonical (sorted unique bound sets), so
	// semantic scalar equality is also the exact retained-representation law.
	// This registration is deliberately separate from skeleton equality: the
	// relation skeleton has a finer representation quotient above.
	ops.scalarRepresentationEqual = ops.scalarEqual
	return ops
}

func overlayDiffRelationCoordinateShapes(keys *keyspace.KeySpace, current, image, selected []diffRelationCoordinateKey) []diffRelationCoordinateKey {
	less := func(left, right diffRelationCoordinateKey) bool {
		return diffRelationCoordinateKeyLess(left, right, keys)
	}
	out := make([]diffRelationCoordinateKey, 0, len(current)+len(selected))
	currentIndex, imageIndex, selectedIndex := 0, 0, 0
	imageHas := func(want diffRelationCoordinateKey) bool {
		for imageIndex < len(image) && less(image[imageIndex], want) {
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
		case less(current[currentIndex], selected[selectedIndex]):
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

func applyDiffRelationCoordinateBranchConstraint(
	skeleton coordinateSkeletonPayload,
	key coordinateKeyPayload,
	current coordinateScalarPayload,
	constraint RelConstraint,
) (coordinateSkeletonPayload, coordinateScalarPayload, bool) {
	canonical, ok := canonicalRelConstraint(constraint)
	shape := diffRelationCoordinateKeyValue(key)
	if !ok {
		return nil, nil, false
	}
	derived, ok := diffRelationCoordinateKeyFromConstraint(diffRelationCoordinateSkeletonValue(skeleton).keys, canonical)
	if !ok || derived != shape {
		return nil, nil, false
	}
	topology := diffRelationCoordinateSkeletonValue(skeleton)
	if topology.keys == nil || !topology.keys.Valid() {
		return nil, nil, false
	}
	if topology.bottom {
		topology = newDiffRelationCoordinateSkeleton(topology.keys, false, nil)
	}
	if !diffRelationShapePresent(topology.shapes, shape, topology.keys) {
		topology = newDiffRelationCoordinateSkeleton(topology.keys, false, mergeDiffRelationShapes(topology.shapes, []diffRelationCoordinateKey{shape}, topology.keys, false))
	}
	scalar := diffRelationCoordinateScalarValue(current)
	if scalar.bottom {
		scalar = diffRelationCoordinateScalar{}
	}
	scalar.bounds = canonicalInt64Set(append(append([]int64(nil), scalar.bounds...), canonical.K))
	return wrapDiffRelationCoordinateSkeleton(topology), wrapDiffRelationCoordinateScalar(scalar), true
}

func diffRelationCoordinateRootAssignment() coordinateRootAssignmentOps {
	ops := noCoordinateRootAssignment()
	ops.scalarTransfer = coordinateScalarTransferOps{
		kind: coordinateScalarTransferParticipant,
		demand: func(inventory []coordinateKeyPayload, transfer RootAssignmentScalarTransfer) ([]coordinateScalarTransferDemand, bool) {
			out := make([]coordinateScalarTransferDemand, 0)
			for _, payload := range inventory {
				shape := diffRelationCoordinateKeyValue(payload)
				for _, operand := range diffRelationCoordinateKeyRelOperands(shape) {
					if operand.Key == transfer.targetState {
						out = append(out, coordinateScalarTransferDemand{target: wrapDiffRelationCoordinateKey(shape)})
						break
					}
				}
			}
			return out, true
		},
		apply: func(currentSkeleton coordinateSkeletonPayload, _ coordinateScalarPayload, _ coordinateScalarPayload, _ bool, _ RootAssignmentScalarTransfer) (coordinateSkeletonPayload, coordinateScalarPayload, bool) {
			return currentSkeleton, wrapDiffRelationCoordinateScalar(diffRelationCoordinateScalar{}), true
		},
	}
	return ops
}

func diffRelationCoordinateBoundaryOps() coordinateFamilyBoundaryOps {
	return coordinateFamilyBoundaryOps{
		admission: coordinateBoundaryAdmissionAllPreimages,
		rootUse:   boundaryRootUseReachability(),
		reachabilityKey: func(program *boundaryReachabilityProgramBuilder, payload coordinateKeyPayload) {
			for _, operand := range diffRelationCoordinateKeyOperands(diffRelationCoordinateKeyValue(payload)) {
				program.pathCone(false, operand.path)
			}
		},
		projectSkeleton: func(ctx *boundaryProjectContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			value := diffRelationCoordinateSkeletonValue(source)
			if value.bottom {
				return source, true
			}
			// The skeleton is the exact support of the scalar relation facts.
			// Projection must therefore remove shapes outside the selected
			// closure before rebase: those operands deliberately have no entry
			// in the boundary quotient. Carrying the full source inventory here
			// made the factor path stricter than the canonical lane projection
			// and attempted to rebase unrelated caller-local state keys.
			projected := make([]diffRelationCoordinateKey, 0, len(value.shapes))
			for _, shape := range value.shapes {
				if diffRelationCoordinateTouches(shape, ctx.closure) {
					projected = append(projected, shape)
				}
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(value.keys, false, projected)), true
		},
		projectKey: func(ctx *boundaryProjectContext, source coordinateKeyPayload) (coordinateKeyPayload, bool, bool) {
			return source, diffRelationCoordinateTouches(diffRelationCoordinateKeyValue(source), ctx.closure), true
		},
		projectScalar: func(_ *boundaryProjectContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		rebaseSkeleton: func(ctx *boundaryRebaseContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			value := diffRelationCoordinateSkeletonValue(source)
			if value.bottom {
				return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{bottom: true, keys: ctx.toKeys}), true
			}
			var out []diffRelationCoordinateKey
			for _, shape := range value.shapes {
				mapped, ok := rebaseDiffRelationCoordinateKey(ctx, shape)
				if !ok {
					return nil, false
				}
				out = append(out, mapped...)
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(ctx.toKeys, false, out)), true
		},
		rebaseKeys: func(ctx *boundaryRebaseContext, source coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			keys, ok := rebaseDiffRelationCoordinateKey(ctx, diffRelationCoordinateKeyValue(source))
			if !ok {
				return nil, false
			}
			out := make([]coordinateKeyPayload, len(keys))
			for i, key := range keys {
				out[i] = wrapDiffRelationCoordinateKey(key)
			}
			return out, true
		},
		rebaseScalar: func(_ *boundaryRebaseContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		sourceFiber: func(source coordinateKeyPayload) coordinateFiberPayload {
			return typedCoordinateFiberPayload[diffRelationCoordinateKey]{value: diffRelationCoordinateKeyValue(source)}
		},
		inverseFibers: func(ctx *boundaryRebaseContext, destination coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
			keys, ok := preimageDiffRelationCoordinateKey(ctx, diffRelationCoordinateKeyValue(destination))
			if !ok {
				return nil, false
			}
			out := make([]coordinateFiberPayload, len(keys))
			for i, key := range keys {
				out[i] = typedCoordinateFiberPayload[diffRelationCoordinateKey]{value: key}
			}
			return out, true
		},
		postEntries: noCoordinatePostEntries,
		applySkeleton: func(_ *boundaryApplyContext, destination, fragment coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			left, right := diffRelationCoordinateSkeletonValue(destination), diffRelationCoordinateSkeletonValue(fragment)
			if left.bottom || right.bottom {
				return wrapDiffRelationCoordinateSkeleton(diffRelationCoordinateSkeleton{bottom: true, keys: left.keys}), true
			}
			return wrapDiffRelationCoordinateSkeleton(newDiffRelationCoordinateSkeleton(left.keys, false, mergeDiffRelationShapes(left.shapes, right.shapes, left.keys, false))), true
		},
		affectedSelector: func(builder *boundaryAffectedSelectorBuilder, key coordinateKeyPayload) {
			operands := diffRelationCoordinateKeyOperands(diffRelationCoordinateKeyValue(key))
			paths := make([]keyspace.Key, 0, len(operands))
			for _, operand := range operands {
				paths = append(paths, operand.path)
			}
			builder.anyPaths(paths...)
		},
		applyScalar: func(_ coordinateKeyPayload, destination, fragment coordinateScalarPayload, affected bool) (coordinateScalarPayload, bool) {
			if affected {
				return fragment, true
			}
			return destination, true
		},
		applyRootSkeleton: func(_ *boundaryApplyContext, source coordinateSkeletonPayload, establishes bool) (coordinateSkeletonPayload, bool) {
			value := diffRelationCoordinateSkeletonValue(source)
			if establishes {
				value.bottom = false
			}
			return wrapDiffRelationCoordinateSkeleton(value), true
		},
		rootSlot: func(_ *boundaryApplyContext, _ BoundaryFactorTarget) (coordinateKeyPayload, bool, bool) {
			return nil, false, true
		},
		rootScalar: func(_ *boundaryApplyContext, _ coordinateKeyPayload, _ product.Value) (coordinateScalarPayload, bool) {
			return nil, false
		},
	}
}

func (d ProductDomain) DiffRelationCoordinateFamily() (CoordinateFamily, bool) {
	lane, ok := d.ProductLane(LaneDiffRelations)
	if !ok {
		return CoordinateFamily{}, false
	}
	families, err := d.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		return CoordinateFamily{}, false
	}
	return families[0], true
}

// DiffRelationShapeComponent closes the exact finite relation-shape component
// from seed operands using skeleton inventory only. No scalar is observed.
func (d ProductDomain) DiffRelationShapeComponent(skeleton CoordinateFamilySkeleton, seeds []RelOperand) ([]CoordinateSlot, error) {
	family, ok := d.DiffRelationCoordinateFamily()
	if skeleton.payload == nil {
		return nil, fmt.Errorf("%w: empty relation skeleton payload", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if !ok {
		return nil, fmt.Errorf("%w: relation family absent", ErrInvalidLaneFactor)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: relation skeleton", err)
	}
	if skeleton.family != family {
		return nil, ErrInvalidLaneFactor
	}
	topology := diffRelationCoordinateSkeletonValue(skeleton.payload)
	if topology.bottom {
		return nil, nil
	}
	seenOperands := make(map[diffRelationCoordinateOperand]struct{})
	queue := make([]diffRelationCoordinateOperand, 0, len(seeds))
	for _, seed := range seeds {
		if !seed.valid() {
			return nil, fmt.Errorf("%w: invalid relation component seed", ErrInvalidLaneFactor)
		}
		operand, valid := diffRelationCoordinateOperandFromRel(topology.keys, seed)
		if !valid {
			return nil, fmt.Errorf("%w: foreign relation component seed", ErrInvalidLaneFactor)
		}
		if _, found := seenOperands[operand]; !found {
			seenOperands[operand] = struct{}{}
			queue = append(queue, operand)
		}
	}
	seenShapes := make(map[diffRelationCoordinateKey]struct{})
	for head := 0; head < len(queue); head++ {
		operand := queue[head]
		for _, shapeIndex := range topology.incidence[operand] {
			shape := topology.shapes[shapeIndex]
			if _, found := seenShapes[shape]; found {
				continue
			}
			seenShapes[shape] = struct{}{}
			for _, next := range diffRelationCoordinateKeyOperands(shape) {
				if _, found := seenOperands[next]; !found {
					seenOperands[next] = struct{}{}
					queue = append(queue, next)
				}
			}
		}
	}
	shapes := make([]diffRelationCoordinateKey, 0, len(seenShapes))
	for shape := range seenShapes {
		shapes = append(shapes, shape)
	}
	sort.Slice(shapes, func(i, j int) bool { return diffRelationCoordinateKeyLess(shapes[i], shapes[j], topology.keys) })
	out := make([]CoordinateSlot, len(shapes))
	for i, shape := range shapes {
		payload := wrapDiffRelationCoordinateKey(shape)
		if !coordinate.ops.keyValid(payload, skeleton.keys) {
			return nil, ErrInvalidLaneFactor
		}
		out[i] = CoordinateSlot{family: family, keys: skeleton.keys, key: payload}
	}
	return out, nil
}

func (d ProductDomain) DiffRelationShapeConstraints(value CoordinateScalarFactor) ([]RelConstraint, bool, error) {
	family, ok := d.DiffRelationCoordinateFamily()
	coordinate, err := d.validateCoordinateFamily(family)
	if !ok || err != nil || value.slot.family != family || d.validateCoordinateFactorFor(coordinate, value, value.slot.keys) != nil {
		return nil, false, ErrInvalidLaneFactor
	}
	scalar := diffRelationCoordinateScalarValue(value.payload)
	if scalar.bottom {
		return nil, false, nil
	}
	key := diffRelationCoordinateKeyValue(value.slot.key)
	out := make([]RelConstraint, 0, len(scalar.bounds))
	for _, bound := range scalar.bounds {
		constraint, valid := diffRelationConstraintFromCoordinate(value.slot.keys, key, bound)
		if !valid {
			return nil, false, ErrInvalidLaneFactor
		}
		out = append(out, constraint)
	}
	return out, len(out) != 0, nil
}

func (c RelConstraint) AppendOperands(out []RelOperand) []RelOperand {
	return appendRelConstraintOperands(out, c)
}

func diffRelationCoordinateKeyFromConstraint(keys *keyspace.KeySpace, c RelConstraint) (diffRelationCoordinateKey, bool) {
	canonical, ok := canonicalRelConstraint(c)
	if !ok {
		return diffRelationCoordinateKey{}, false
	}
	a, aok := diffRelationCoordinateOperandFromRel(keys, canonical.A)
	b := diffRelationCoordinateOperand{}
	bok := true
	if canonical.B.valid() {
		b, bok = diffRelationCoordinateOperandFromRel(keys, canonical.B)
	}
	cc, cok := diffRelationCoordinateOperandFromRel(keys, canonical.C)
	return diffRelationCoordinateKey{coA: canonical.CoA, a: a, coB: canonical.CoB, b: b, c: cc}, aok && bok && cok
}
func diffRelationConstraintFromCoordinate(keys *keyspace.KeySpace, key diffRelationCoordinateKey, bound int64) (RelConstraint, bool) {
	a, aok := diffRelationRelOperand(keys, key.a)
	c, cok := diffRelationRelOperand(keys, key.c)
	var b RelOperand
	bok := true
	if key.coB != 0 {
		b, bok = diffRelationRelOperand(keys, key.b)
	}
	constraint, ok := canonicalRelConstraint(RelConstraint{CoA: key.coA, A: a, CoB: key.coB, B: b, C: c, K: bound})
	return constraint, aok && bok && cok && ok
}
func diffRelationCoordinateOperandFromRel(keys *keyspace.KeySpace, operand RelOperand) (diffRelationCoordinateOperand, bool) {
	if !operand.valid() || keys == nil || !keys.Valid() {
		return diffRelationCoordinateOperand{}, false
	}
	path, ok := keys.InternStateKey(operand.Key)
	return diffRelationCoordinateOperand{path: path, kind: operand.Kind, state: operand.Key}, ok
}
func diffRelationRelOperand(keys *keyspace.KeySpace, operand diffRelationCoordinateOperand) (RelOperand, bool) {
	state, ok := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(operand.path))
	return RelOperand{Key: state, Kind: operand.kind}, ok && state == operand.state
}
func diffRelationCoordinateKeyValid(key diffRelationCoordinateKey, keys *keyspace.KeySpace) bool {
	_, ok := diffRelationConstraintFromCoordinate(keys, key, 0)
	return ok
}
func diffRelationCoordinateKeyOperands(key diffRelationCoordinateKey) []diffRelationCoordinateOperand {
	out := []diffRelationCoordinateOperand{key.a}
	if key.coB != 0 {
		out = append(out, key.b)
	}
	out = append(out, key.c)
	return out
}
func diffRelationCoordinateKeyRelOperands(key diffRelationCoordinateKey) []RelOperand {
	out := make([]RelOperand, 0, 3)
	for _, operand := range diffRelationCoordinateKeyOperands(key) {
		candidate := RelOperand{Key: operand.state, Kind: operand.kind}
		duplicate := false
		for _, prior := range out {
			if prior == candidate {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, candidate)
		}
	}
	return out
}
func diffRelationCoordinateKeyLess(a, b diffRelationCoordinateKey, keys *keyspace.KeySpace) bool {
	if a.coA != b.coA {
		return a.coA < b.coA
	}
	if diffRelationCoordinateOperandLess(a.a, b.a, keys) {
		return true
	}
	if a.a != b.a {
		return false
	}
	if a.coB != b.coB {
		return a.coB < b.coB
	}
	if diffRelationCoordinateOperandLess(a.b, b.b, keys) {
		return true
	}
	if a.b != b.b {
		return false
	}
	return diffRelationCoordinateOperandLess(a.c, b.c, keys)
}
func diffRelationCoordinateOperandLess(a, b diffRelationCoordinateOperand, keys *keyspace.KeySpace) bool {
	if a.path != b.path {
		return keys.Less(a.path, b.path)
	}
	return a.kind < b.kind
}

func appendRelConstraintOperands(out []RelOperand, constraint RelConstraint) []RelOperand {
	for _, operand := range []RelOperand{constraint.A, constraint.B, constraint.C} {
		if !operand.valid() {
			continue
		}
		duplicate := false
		for _, prior := range out {
			if prior == operand {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, operand)
		}
	}
	return out
}
func canonicalInt64Set(in []int64) []int64 {
	out := append([]int64(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	n := 0
	for _, v := range out {
		if n == 0 || out[n-1] != v {
			out[n] = v
			n++
		}
	}
	return out[:n]
}
func int64SetCanonical(in []int64) bool {
	for i := 1; i < len(in); i++ {
		if in[i-1] >= in[i] {
			return false
		}
	}
	return true
}
func equalDiffRelationCoordinateScalar(a, b diffRelationCoordinateScalar) bool {
	if a.bottom != b.bottom || len(a.bounds) != len(b.bounds) {
		return false
	}
	for i := range a.bounds {
		if a.bounds[i] != b.bounds[i] {
			return false
		}
	}
	return true
}
func lessOrEqDiffRelationCoordinateScalar(a, b diffRelationCoordinateScalar) bool {
	if a.bottom {
		return true
	}
	if b.bottom {
		return false
	}
	i := 0
	for _, want := range b.bounds {
		for i < len(a.bounds) && a.bounds[i] < want {
			i++
		}
		if i == len(a.bounds) || a.bounds[i] != want {
			return false
		}
		i++
	}
	return true
}
func joinDiffRelationCoordinateScalar(a, b diffRelationCoordinateScalar) diffRelationCoordinateScalar {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	out := make([]int64, 0)
	i, j := 0, 0
	for i < len(a.bounds) && j < len(b.bounds) {
		if a.bounds[i] < b.bounds[j] {
			i++
		} else if b.bounds[j] < a.bounds[i] {
			j++
		} else {
			out = append(out, a.bounds[i])
			i++
			j++
		}
	}
	return diffRelationCoordinateScalar{bounds: out}
}
func meetDiffRelationCoordinateScalar(a, b diffRelationCoordinateScalar) diffRelationCoordinateScalar {
	if a.bottom || b.bottom {
		return diffRelationCoordinateScalar{bottom: true}
	}
	return diffRelationCoordinateScalar{bounds: canonicalInt64Set(append(append([]int64(nil), a.bounds...), b.bounds...))}
}

func newDiffRelationCoordinateSkeleton(keys *keyspace.KeySpace, bottom bool, shapes []diffRelationCoordinateKey) diffRelationCoordinateSkeleton {
	out := append([]diffRelationCoordinateKey(nil), shapes...)
	sort.Slice(out, func(i, j int) bool { return diffRelationCoordinateKeyLess(out[i], out[j], keys) })
	n := 0
	for _, shape := range out {
		if n == 0 || out[n-1] != shape {
			out[n] = shape
			n++
		}
	}
	out = out[:n]
	incidence := make(map[diffRelationCoordinateOperand][]int)
	for index, shape := range out {
		seen := make(map[diffRelationCoordinateOperand]struct{}, 3)
		for _, operand := range diffRelationCoordinateKeyOperands(shape) {
			if _, duplicate := seen[operand]; duplicate {
				continue
			}
			seen[operand] = struct{}{}
			incidence[operand] = append(incidence[operand], index)
		}
	}
	return diffRelationCoordinateSkeleton{bottom: bottom, keys: keys, shapes: out, incidence: incidence}
}

func diffRelationIncidenceValid(skeleton diffRelationCoordinateSkeleton) bool {
	want := newDiffRelationCoordinateSkeleton(skeleton.keys, skeleton.bottom, skeleton.shapes).incidence
	if len(want) != len(skeleton.incidence) {
		return false
	}
	for operand, indexes := range want {
		got, ok := skeleton.incidence[operand]
		if !ok || len(got) != len(indexes) {
			return false
		}
		for i := range indexes {
			if got[i] != indexes[i] {
				return false
			}
		}
	}
	return true
}
func diffRelationShapesCanonical(shapes []diffRelationCoordinateKey, keys *keyspace.KeySpace) bool {
	for i, shape := range shapes {
		if !diffRelationCoordinateKeyValid(shape, keys) || i > 0 && !diffRelationCoordinateKeyLess(shapes[i-1], shape, keys) {
			return false
		}
	}
	return true
}
func diffRelationShapesEqual(a, b []diffRelationCoordinateKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func mergeDiffRelationShapes(a, b []diffRelationCoordinateKey, keys *keyspace.KeySpace, intersect bool) []diffRelationCoordinateKey {
	if intersect {
		out := make([]diffRelationCoordinateKey, 0)
		i, j := 0, 0
		for i < len(a) && j < len(b) {
			if diffRelationCoordinateKeyLess(a[i], b[j], keys) {
				i++
			} else if diffRelationCoordinateKeyLess(b[j], a[i], keys) {
				j++
			} else {
				out = append(out, a[i])
				i++
				j++
			}
		}
		return out
	}
	return newDiffRelationCoordinateSkeleton(keys, false, append(append([]diffRelationCoordinateKey(nil), a...), b...)).shapes
}
func diffRelationShapePresent(shapes []diffRelationCoordinateKey, key diffRelationCoordinateKey, keys *keyspace.KeySpace) bool {
	i := sort.Search(len(shapes), func(i int) bool { return !diffRelationCoordinateKeyLess(shapes[i], key, keys) })
	return i < len(shapes) && shapes[i] == key
}

func wrapDiffRelationCoordinateSkeleton(v diffRelationCoordinateSkeleton) coordinateSkeletonPayload {
	v.shapes = append([]diffRelationCoordinateKey(nil), v.shapes...)
	if v.incidence != nil {
		cloned := make(map[diffRelationCoordinateOperand][]int, len(v.incidence))
		for operand, indexes := range v.incidence {
			cloned[operand] = append([]int(nil), indexes...)
		}
		v.incidence = cloned
	}
	return typedCoordinateSkeletonPayload[diffRelationCoordinateSkeleton]{value: v}
}
func diffRelationCoordinateSkeletonValue(p coordinateSkeletonPayload) diffRelationCoordinateSkeleton {
	v, ok := p.(typedCoordinateSkeletonPayload[diffRelationCoordinateSkeleton])
	if !ok {
		panic("state: relation coordinate skeleton payload mismatch")
	}
	return v.value
}
func wrapDiffRelationCoordinateKey(v diffRelationCoordinateKey) coordinateKeyPayload {
	return typedCoordinateKeyPayload[diffRelationCoordinateKey]{value: v}
}
func diffRelationCoordinateKeyValue(p coordinateKeyPayload) diffRelationCoordinateKey {
	v, ok := p.(typedCoordinateKeyPayload[diffRelationCoordinateKey])
	if !ok {
		panic("state: relation coordinate key payload mismatch")
	}
	return v.value
}
func wrapDiffRelationCoordinateScalar(v diffRelationCoordinateScalar) coordinateScalarPayload {
	v.bounds = append([]int64(nil), v.bounds...)
	return typedCoordinateScalarPayload[diffRelationCoordinateScalar]{value: v}
}
func diffRelationCoordinateScalarValue(p coordinateScalarPayload) diffRelationCoordinateScalar {
	v, ok := p.(typedCoordinateScalarPayload[diffRelationCoordinateScalar])
	if !ok {
		panic("state: relation coordinate scalar payload mismatch")
	}
	return v.value
}

func importDiffRelationCoordinateKey(source diffRelationCoordinateKey, from, to *keyspace.KeySpace) (diffRelationCoordinateKey, bool) {
	constraint, ok := diffRelationConstraintFromCoordinate(from, source, 0)
	if !ok {
		return diffRelationCoordinateKey{}, false
	}
	mapOperand := func(operand RelOperand) (RelOperand, bool) {
		old, exists := from.InternStateKey(operand.Key)
		if !exists {
			return RelOperand{}, false
		}
		next, exists := to.ImportKey(from, old)
		if !exists {
			return RelOperand{}, false
		}
		state, stateOK := pathaddr.StateKeyFromPathKey(to.FormatReadOnly(next))
		operand.Key = state
		return operand, stateOK
	}
	constraint.A, ok = mapOperand(constraint.A)
	if !ok {
		return diffRelationCoordinateKey{}, false
	}
	if constraint.B.valid() {
		constraint.B, ok = mapOperand(constraint.B)
		if !ok {
			return diffRelationCoordinateKey{}, false
		}
	}
	constraint.C, ok = mapOperand(constraint.C)
	if !ok {
		return diffRelationCoordinateKey{}, false
	}
	return diffRelationCoordinateKeyFromConstraint(to, constraint)
}

func mapDiffRelationCoordinateKey(source diffRelationCoordinateKey, to *keyspace.KeySpace, mapKey func(keyspace.Key) (keyspace.Key, bool)) (diffRelationCoordinateKey, bool) {
	if to == nil || !to.Valid() || mapKey == nil {
		return diffRelationCoordinateKey{}, false
	}
	mapOperand := func(source diffRelationCoordinateOperand) (diffRelationCoordinateOperand, bool) {
		if source.kind == 0 {
			return source, true
		}
		path, ok := mapKey(source.path)
		if !ok {
			return diffRelationCoordinateOperand{}, false
		}
		state, ok := pathaddr.StateKeyFromPathKey(to.FormatReadOnly(path))
		if !ok {
			return diffRelationCoordinateOperand{}, false
		}
		return diffRelationCoordinateOperand{path: path, kind: source.kind, state: state}, true
	}
	var ok bool
	source.a, ok = mapOperand(source.a)
	if !ok {
		return diffRelationCoordinateKey{}, false
	}
	if source.coB != 0 {
		source.b, ok = mapOperand(source.b)
		if !ok {
			return diffRelationCoordinateKey{}, false
		}
	}
	source.c, ok = mapOperand(source.c)
	if !ok || !diffRelationCoordinateKeyValid(source, to) {
		return diffRelationCoordinateKey{}, false
	}
	return source, true
}
func hashDiffRelationCoordinateKey(key diffRelationCoordinateKey, keys *keyspace.KeySpace) uint64 {
	h := internal.MixHash(0, uint64(key.coA))
	h = hashDiffRelationOperand(h, key.a, keys)
	h = internal.MixHash(h, uint64(key.coB))
	h = hashDiffRelationOperand(h, key.b, keys)
	return hashDiffRelationOperand(h, key.c, keys)
}
func hashDiffRelationOperand(h uint64, operand diffRelationCoordinateOperand, keys *keyspace.KeySpace) uint64 {
	h = internal.MixHash(h, uint64(operand.kind))
	if operand.kind != 0 {
		h = internal.MixHash(h, internal.FnvString(string(keys.FormatReadOnly(operand.path))))
	}
	return h
}
func diffRelationCoordinateTouches(key diffRelationCoordinateKey, closure BoundaryClosure) bool {
	for _, operand := range diffRelationCoordinateKeyOperands(key) {
		if closure.ContainsPath(operand.path) {
			return true
		}
	}
	return false
}
func rebaseDiffRelationCoordinateKey(ctx *boundaryRebaseContext, key diffRelationCoordinateKey) ([]diffRelationCoordinateKey, bool) {
	constraint, ok := diffRelationConstraintFromCoordinate(ctx.fromKeys, key, 0)
	if !ok {
		return nil, false
	}
	mapped, ok := rebaseRelConstraint(ctx, constraint)
	if !ok {
		return nil, false
	}
	out := make([]diffRelationCoordinateKey, 0, len(mapped))
	for _, next := range mapped {
		shape, valid := diffRelationCoordinateKeyFromConstraint(ctx.toKeys, next)
		if !valid {
			return nil, false
		}
		out = append(out, shape)
	}
	return newDiffRelationCoordinateSkeleton(ctx.toKeys, false, out).shapes, true
}
func preimageDiffRelationCoordinateKey(ctx *boundaryRebaseContext, key diffRelationCoordinateKey) ([]diffRelationCoordinateKey, bool) {
	constraint, ok := diffRelationConstraintFromCoordinate(ctx.toKeys, key, 0)
	if !ok {
		return nil, false
	}
	preimages := func(operand RelOperand) ([]RelOperand, bool) {
		if !operand.valid() {
			return []RelOperand{operand}, true
		}
		keys, valid := ctx.quotient.stateKeyPreimages(operand.Key)
		if !valid {
			return nil, false
		}
		out := make([]RelOperand, len(keys))
		for i, state := range keys {
			out[i] = operand
			out[i].Key = state
		}
		return out, true
	}
	as, aok := preimages(constraint.A)
	bs, bok := preimages(constraint.B)
	cs, cok := preimages(constraint.C)
	if !aok || !bok || !cok {
		return nil, false
	}
	var out []diffRelationCoordinateKey
	for _, a := range as {
		for _, b := range bs {
			for _, c := range cs {
				source := constraint
				source.A, source.B, source.C = a, b, c
				source, valid := canonicalRelConstraint(source)
				if !valid {
					continue
				}
				mapped, valid := rebaseRelConstraint(ctx, source)
				if !valid {
					return nil, false
				}
				for _, candidate := range mapped {
					if candidate == constraint {
						shape, shapeOK := diffRelationCoordinateKeyFromConstraint(ctx.fromKeys, source)
						if shapeOK {
							out = append(out, shape)
						}
						break
					}
				}
			}
		}
	}
	return newDiffRelationCoordinateSkeleton(ctx.fromKeys, false, out).shapes, true
}
