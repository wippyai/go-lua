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
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

const placementCoordinateFamilyID CoordinateFamilyID = "identity-placement"

type placementCoordinateSkeleton struct{ top bool }
type placementCoordinateKey struct{ id identity.Term }
type placementCoordinateScalar struct{ value placement.Value }

var placementCoordinateFamilySpec = coordinateFamilySpec{
	dynamicRead:   dynamicReadCoordinateIndependent(),
	identityImage: IdentityImagePointwiseMap,
	id:            placementCoordinateFamilyID,
	build:         buildPlacementCoordinateFamily,
	boundary: coordinateFamilyBoundaryOps{
		admission:       coordinateBoundaryAdmissionAnyPresent,
		rootUse:         boundaryRootUseNone(),
		reachabilityKey: func(*boundaryReachabilityProgramBuilder, coordinateKeyPayload) {},
		projectSkeleton: func(_ *boundaryProjectContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			return source, true
		},
		projectKey: func(ctx *boundaryProjectContext, source coordinateKeyPayload) (coordinateKeyPayload, bool, bool) {
			key := placementCoordinateKeyValue(source)
			return source, ctx.closure.ContainsIdentityTerm(key.id), true
		},
		projectScalar: func(_ *boundaryProjectContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		rebaseSkeleton: func(_ *boundaryRebaseContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			return source, true
		},
		rebaseKeys: func(ctx *boundaryRebaseContext, source coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			id := placementCoordinateKeyValue(source).id
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
			return []coordinateKeyPayload{wrapPlacementCoordinateKey(id)}, true
		},
		rebaseScalar: func(_ *boundaryRebaseContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return source, true
		},
		sourceFiber: func(source coordinateKeyPayload) coordinateFiberPayload {
			return typedCoordinateFiberPayload[identity.Term]{value: placementCoordinateKeyValue(source).id}
		},
		inverseFibers: func(ctx *boundaryRebaseContext, destination coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
			ids, ok := ctx.quotient.identityPreimages(placementCoordinateKeyValue(destination).id)
			if !ok {
				return nil, false
			}
			out := make([]coordinateFiberPayload, len(ids))
			for index, id := range ids {
				out[index] = typedCoordinateFiberPayload[identity.Term]{value: id}
			}
			return out, true
		},
		postEntries: noCoordinatePostEntries,
		applySkeleton: func(_ *boundaryApplyContext, destination, fragment coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			left := placementCoordinateSkeletonValue(destination)
			right := placementCoordinateSkeletonValue(fragment)
			return wrapPlacementCoordinateSkeleton(placementCoordinateSkeleton{top: left.top || right.top}), true
		},
		applyScalar: func(_ coordinateKeyPayload, destination, fragment coordinateScalarPayload, affected bool) (coordinateScalarPayload, bool) {
			if affected {
				return fragment, true
			}
			return destination, true
		},
		affectedSelector: func(builder *boundaryAffectedSelectorBuilder, key coordinateKeyPayload) {
			builder.anyIdentities(placementCoordinateKeyValue(key).id)
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

func buildPlacementCoordinateFamily(_ *axis.Registry, _ DomainOptions) coordinateFamilyOps {
	domain := placementMapDomain()
	return withSemanticSkeletonRepresentation(coordinateFamilyOps{
		branchRelation:      noCoordinateBranchRelation(),
		inventoryCompletion: noCoordinateInventoryCompletions(),
		requiredScalarKeys:  func(coordinateSkeletonPayload) []coordinateKeyPayload { return nil },
		sealSkeletonInventory: func(skeleton coordinateSkeletonPayload, _ []coordinateKeyPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, bool) {
			if keys == nil || !keys.Valid() {
				return nil, nil, false
			}
			return skeleton, nil, true
		},
		decompose: func(payload laneFactorPayload, _ *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, error) {
			lane := typedLaneFactorValue[placementLane](payload)
			skeleton := wrapPlacementCoordinateSkeleton(placementCoordinateSkeleton{top: lane.top})
			if lane.top {
				return skeleton, nil, nil
			}
			ids := make([]identity.Term, 0, len(lane.values))
			for id, value := range lane.values {
				if !id.Valid() || value <= placement.Bottom || value > placement.Unknown {
					return nil, nil, fmt.Errorf("invalid placement coordinate")
				}
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool { return identityTermLess(ids[i], ids[j]) })
			entries := make([]coordinateEntry, len(ids))
			for index, id := range ids {
				entries[index] = coordinateEntry{
					key:    wrapPlacementCoordinateKey(id),
					scalar: wrapPlacementCoordinateScalar(lane.values[id]),
				}
			}
			return skeleton, entries, nil
		},
		replace: func(_ laneFactorPayload, _ *keyspace.KeySpace, skeleton coordinateSkeletonPayload, entries []coordinateEntry) (laneFactorPayload, error) {
			if placementCoordinateSkeletonValue(skeleton).top {
				if len(entries) != 0 {
					return nil, fmt.Errorf("placement Top cannot carry finite coordinates")
				}
				return typedLaneFactorPayload[placementLane]{value: placementLane{mapLane: mapLane[identity.Term, placement.Value]{top: true}}}, nil
			}
			values := make(map[identity.Term]placement.Value, len(entries))
			for index, entry := range entries {
				key := placementCoordinateKeyValue(entry.key)
				scalar := placementCoordinateScalarValue(entry.scalar)
				if !key.id.Valid() || scalar.value <= placement.Bottom || scalar.value > placement.Unknown {
					return nil, fmt.Errorf("invalid placement coordinate %d", index)
				}
				if _, duplicate := values[key.id]; duplicate {
					return nil, fmt.Errorf("duplicate placement coordinate")
				}
				values[key.id] = scalar.value
			}
			return typedLaneFactorPayload[placementLane]{value: placementLaneFromMap(domain, values)}, nil
		},

		skeletonBottom: func() coordinateSkeletonPayload {
			return wrapPlacementCoordinateSkeleton(placementCoordinateSkeleton{})
		},
		skeletonTop: func() coordinateSkeletonPayload {
			return wrapPlacementCoordinateSkeleton(placementCoordinateSkeleton{top: true})
		},
		skeletonEqual: func(a, b coordinateSkeletonPayload) bool {
			return placementCoordinateSkeletonValue(a) == placementCoordinateSkeletonValue(b)
		},
		skeletonLessOrEq: func(a, b coordinateSkeletonPayload) bool {
			left, right := placementCoordinateSkeletonValue(a), placementCoordinateSkeletonValue(b)
			return !left.top || right.top
		},
		skeletonJoin: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			left, right := placementCoordinateSkeletonValue(a), placementCoordinateSkeletonValue(b)
			return wrapPlacementCoordinateSkeleton(placementCoordinateSkeleton{top: left.top || right.top})
		},
		skeletonMeet: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			left, right := placementCoordinateSkeletonValue(a), placementCoordinateSkeletonValue(b)
			return wrapPlacementCoordinateSkeleton(placementCoordinateSkeleton{top: left.top && right.top})
		},
		skeletonWiden: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			left, right := placementCoordinateSkeletonValue(a), placementCoordinateSkeletonValue(b)
			return wrapPlacementCoordinateSkeleton(placementCoordinateSkeleton{top: left.top || right.top})
		},
		skeletonNarrow: func(previous, _ coordinateSkeletonPayload) coordinateSkeletonPayload { return previous },
		skeletonHash: func(value coordinateSkeletonPayload) uint64 {
			if placementCoordinateSkeletonValue(value).top {
				return 1
			}
			return 0
		},
		importSkeleton: func(source coordinateSkeletonPayload, _, _ *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			return wrapPlacementCoordinateSkeleton(placementCoordinateSkeletonValue(source)), true
		},

		keyValid: func(key coordinateKeyPayload, _ *keyspace.KeySpace) bool {
			return placementCoordinateKeyValue(key).id.Valid()
		},
		keyEqual: func(a, b coordinateKeyPayload) bool {
			return placementCoordinateKeyValue(a) == placementCoordinateKeyValue(b)
		},
		keyLess: func(a, b coordinateKeyPayload, _ *keyspace.KeySpace) bool {
			return identityTermLess(placementCoordinateKeyValue(a).id, placementCoordinateKeyValue(b).id)
		},
		keyHash: func(key coordinateKeyPayload, _ *keyspace.KeySpace) uint64 {
			id := placementCoordinateKeyValue(key).id
			return internal.MixHash(internal.FnvString("placement.coordinate"), id.Hash())
		},
		importKey: func(source coordinateKeyPayload, _, _ *keyspace.KeySpace) (coordinateKeyPayload, bool) {
			key := placementCoordinateKeyValue(source)
			return wrapPlacementCoordinateKey(key.id), key.id.Valid()
		},
		formalRekey:            keyIndependentCoordinateFormalRekey(),
		visitValueDependencies: func(coordinateKeyPayload, *keyspace.KeySpace, func(statekey.ValueDependency)) {},

		defaultScalar: func(skeleton coordinateSkeletonPayload, _ coordinateKeyPayload) (coordinateScalarPayload, error) {
			if placementCoordinateSkeletonValue(skeleton).top {
				return wrapPlacementCoordinateScalar(placement.Unknown), nil
			}
			return wrapPlacementCoordinateScalar(placement.Bottom), nil
		},
		// A returned-identity publication establishes reachability for its
		// exact key without consulting global placement topology. The omitted
		// reachable scalar is therefore the lattice Bottom; declaring it here
		// lets formal D update each publisher root independently.
		reachableDefault: func(coordinateKeyPayload) (coordinateScalarPayload, bool) {
			return wrapPlacementCoordinateScalar(placement.Bottom), true
		},
		scalarSupport: func(skeleton coordinateSkeletonPayload, _ coordinateKeyPayload) CoordinateScalarSupport {
			if !placementCoordinateSkeletonValue(skeleton).top {
				return CoordinateScalarOptional
			}
			return CoordinateScalarForbidden
		},
		scalarValid: func(_ coordinateKeyPayload, scalar coordinateScalarPayload) bool {
			value := placementCoordinateScalarValue(scalar).value
			return value >= placement.Bottom && value <= placement.Unknown
		},
		scalarEqual: func(a, b coordinateScalarPayload) bool {
			return placement.Equal(placementCoordinateScalarValue(a).value, placementCoordinateScalarValue(b).value)
		},
		scalarLessOrEq: func(a, b coordinateScalarPayload) bool {
			return placement.LessOrEq(placementCoordinateScalarValue(a).value, placementCoordinateScalarValue(b).value)
		},
		scalarJoin: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapPlacementCoordinateScalar(placement.Join(placementCoordinateScalarValue(a).value, placementCoordinateScalarValue(b).value))
		},
		scalarMeet: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapPlacementCoordinateScalar(placement.Meet(placementCoordinateScalarValue(a).value, placementCoordinateScalarValue(b).value))
		},
		scalarWiden: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapPlacementCoordinateScalar(placement.Widen(placementCoordinateScalarValue(a).value, placementCoordinateScalarValue(b).value))
		},
		scalarNarrow: func(previous, _ coordinateScalarPayload) coordinateScalarPayload { return previous },
		scalarHash: func(value coordinateScalarPayload) uint64 {
			return placementCoordinateScalarValue(value).value.Hash()
		},
		importScalar: func(source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			value := placementCoordinateScalarValue(source).value
			return wrapPlacementCoordinateScalar(value), value >= placement.Bottom && value <= placement.Unknown
		},
		returnIdentity: coordinateReturnIdentityOps{
			roles: coordinateReturnIdentityRoles(CoordinateReturnIdentityPublisher),
			visitInventoryTerms: func(key coordinateKeyPayload, visit func(identity.Term) bool) bool {
				term := placementCoordinateKeyValue(key).id
				return !term.Valid() || visit(term)
			},
			visitTermKeys: func(term identity.Term, visit func(coordinateKeyPayload) bool) bool {
				if !term.Valid() {
					return false
				}
				return visit(wrapPlacementCoordinateKey(term))
			},
			imageInventoryKey: func(key coordinateKeyPayload, image *CoordinateIdentityTermImage, emit func(coordinateKeyPayload) bool) bool {
				terms, ok := image.Image(placementCoordinateKeyValue(key).id)
				if !ok {
					return false
				}
				for _, term := range terms {
					if !emit(wrapPlacementCoordinateKey(term)) {
						return false
					}
				}
				return true
			},
			visitSeedRoots:     func(coordinateSkeletonPayload, func(identity.Term) bool) {},
			visitSkeletonEdges: func(coordinateSkeletonPayload, func(identity.Term, identity.Term) bool) {},
			visitScalarEdges:   func(coordinateKeyPayload, coordinateScalarPayload, func(identity.Term, identity.Term) bool) {},
			containerScalar: func(coordinateKeyPayload, coordinateScalarPayload) (identity.Term, product.Value, bool) {
				return identity.Term{}, product.Value{}, false
			},
			visitContainerFacts: func(coordinateSkeletonPayload, identity.Term, func(dynamicindex.Fact)) bool { return false },
			publicationKey: func(term identity.Term) (coordinateKeyPayload, bool) {
				if !term.Valid() {
					return nil, false
				}
				return wrapPlacementCoordinateKey(term), true
			},
			publishScalar: func(key coordinateKeyPayload, scalar coordinateScalarPayload, target placement.Value) (coordinateScalarPayload, bool) {
				if !placementCoordinateKeyValue(key).id.Valid() {
					return nil, false
				}
				if target <= placement.Bottom || target > placement.Unknown {
					return nil, false
				}
				value := placement.Join(placementCoordinateScalarValue(scalar).value, target)
				return wrapPlacementCoordinateScalar(value), true
			},
		},
		pathEvidence:   noCoordinatePathEvidence(),
		pathValues:     noCoordinatePathValues(),
		rootAssignment: noCoordinateRootAssignment(),
		pathMutation:   noCoordinatePathMutation(),
		objectMutation: placementCoordinateObjectMutation(),
	})
}

func placementCoordinateSkeletonValue(payload coordinateSkeletonPayload) placementCoordinateSkeleton {
	typed, ok := payload.(typedCoordinateSkeletonPayload[placementCoordinateSkeleton])
	if !ok {
		panic("state: placement coordinate skeleton mismatch")
	}
	return typed.value
}

func placementCoordinateKeyValue(payload coordinateKeyPayload) placementCoordinateKey {
	typed, ok := payload.(typedCoordinateKeyPayload[placementCoordinateKey])
	if !ok {
		panic("state: placement coordinate key mismatch")
	}
	return typed.value
}

func placementCoordinateScalarValue(payload coordinateScalarPayload) placementCoordinateScalar {
	typed, ok := payload.(typedCoordinateScalarPayload[placementCoordinateScalar])
	if !ok {
		panic("state: placement coordinate scalar mismatch")
	}
	return typed.value
}

func wrapPlacementCoordinateSkeleton(value placementCoordinateSkeleton) coordinateSkeletonPayload {
	return typedCoordinateSkeletonPayload[placementCoordinateSkeleton]{value: value}
}

func wrapPlacementCoordinateKey(id identity.Term) coordinateKeyPayload {
	return typedCoordinateKeyPayload[placementCoordinateKey]{value: placementCoordinateKey{id: id}}
}

func wrapPlacementCoordinateScalar(value placement.Value) coordinateScalarPayload {
	return typedCoordinateScalarPayload[placementCoordinateScalar]{value: placementCoordinateScalar{value: value}}
}
