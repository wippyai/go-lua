package state

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

const pathEvidenceCoordinateFamilyID CoordinateFamilyID = "coupled-path-evidence"

type pathEvidenceCoordinateOverlayPlan []pathevidence.CoordinateKey

var pathEvidenceCoordinateFamilySpec = coordinateFamilySpec{
	dynamicRead:   dynamicReadPathCoordinates(),
	identityImage: IdentityImageEmbeddedValue,
	id:            pathEvidenceCoordinateFamilyID,
	build:         buildPathEvidenceCoordinateFamily,
	boundary: coordinateFamilyBoundaryOps{
		admission: coordinateBoundaryAdmissionAllPreimages,
		rootUse:   boundaryRootUsePathValuesAndReachability(),
		reachabilityKey: func(program *boundaryReachabilityProgramBuilder, source coordinateKeyPayload) {
			pathevidence.ExpandCoordinateClosure(pathEvidenceCoordinateKey(source), func(paths ...keyspace.Key) bool {
				return program.pathCone(false, paths...)
			}, program.addValue)
		},
		projectSkeleton: func(_ *boundaryProjectContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			return source, true
		},
		projectKey: func(ctx *boundaryProjectContext, key coordinateKeyPayload) (coordinateKeyPayload, bool, bool) {
			mappedKey, keep := pathevidence.ProjectCoordinateKey(pathEvidenceCoordinateKey(key), ctx.closure.ContainsPath, func(value product.Value) product.Value {
				return product.ProjectBoundary(ctx.reg, value)
			})
			if !keep {
				return nil, false, true
			}
			return typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: mappedKey}, true, true
		},
		projectScalar: func(ctx *boundaryProjectContext, _ coordinateKeyPayload, scalar coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return wrapPathEvidenceCoordinateScalar(pathevidence.ProjectCoordinateScalar(pathEvidenceCoordinateScalar(scalar), func(value product.Value) product.Value {
				return product.ProjectBoundary(ctx.reg, value)
			})), true
		},
		rebaseSkeleton: func(_ *boundaryRebaseContext, source coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			return source, true
		},
		rebaseKeys: func(ctx *boundaryRebaseContext, source coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			mapped, ok := pathevidence.RebaseCoordinateKey(pathEvidenceCoordinateKey(source), func(path keyspace.Key) ([]keyspace.Key, bool) {
				return boundaryRebasePaths(ctx, path)
			}, func(value product.Value) (product.Value, bool) {
				return rebaseBoundaryProduct(ctx, value)
			})
			if !ok {
				return nil, false
			}
			out := make([]coordinateKeyPayload, len(mapped))
			for index, value := range mapped {
				out[index] = typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: value}
			}
			return out, true
		},
		rebaseScalar: func(ctx *boundaryRebaseContext, _ coordinateKeyPayload, source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			mapped, ok := pathevidence.RebaseCoordinateScalar(pathEvidenceCoordinateScalar(source), func(value product.Value) (product.Value, bool) {
				return rebaseBoundaryProduct(ctx, value)
			})
			return wrapPathEvidenceCoordinateScalar(mapped), ok
		},
		sourceFiber: func(source coordinateKeyPayload) coordinateFiberPayload {
			return typedCoordinateFiberPayload[pathevidence.CoordinateFiber]{value: pathevidence.CoordinateSourceFiber(pathEvidenceCoordinateKey(source))}
		},
		inverseFibers: func(ctx *boundaryRebaseContext, destination coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
			fibers, ok := pathevidence.CoordinateInverseFibers(pathEvidenceCoordinateKey(destination), func(path keyspace.Key) ([]keyspace.Key, bool) {
				return ctx.quotient.pathPreimages(path)
			})
			if !ok {
				return nil, false
			}
			out := make([]coordinateFiberPayload, len(fibers))
			for index, fiber := range fibers {
				out[index] = typedCoordinateFiberPayload[pathevidence.CoordinateFiber]{value: fiber}
			}
			return out, true
		},
		postEntries: func(aliases [][2]keyspace.Key) []coordinateEntry {
			additions := pathevidence.BoundaryAliasCoordinateEntries(aliases)
			out := make([]coordinateEntry, len(additions))
			for index, addition := range additions {
				out[index] = coordinateEntry{key: typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: addition.Key}, scalar: wrapPathEvidenceCoordinateScalar(addition.Scalar)}
			}
			return out
		},
		applySkeleton: applyPathEvidenceCoordinateSkeleton,
		affectedSelector: func(builder *boundaryAffectedSelectorBuilder, key coordinateKeyPayload) {
			pathevidence.CoordinateKeyTouches(pathEvidenceCoordinateKey(key), func(path keyspace.Key) bool {
				builder.anyPaths(path)
				return false
			})
			builder.neverIfEmpty()
		},
		applyScalar: applyPathEvidenceCoordinateScalar,
		applyRootSkeleton: func(_ *boundaryApplyContext, skeleton coordinateSkeletonPayload, establishes bool) (coordinateSkeletonPayload, bool) {
			return wrapPathEvidenceCoordinateSkeleton(pathevidence.ApplyCoordinateRootReachability(pathEvidenceCoordinateSkeleton(skeleton), establishes)), true
		},
		rootSlot: func(_ *boundaryApplyContext, target BoundaryFactorTarget) (coordinateKeyPayload, bool, bool) {
			key, claimed := pathevidence.BoundaryRootSlot(target.Path)
			if !claimed {
				return nil, false, true
			}
			return typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: key}, true, true
		},
		rootScalar: func(ctx *boundaryApplyContext, key coordinateKeyPayload, value product.Value) (coordinateScalarPayload, bool) {
			scalar, ok := pathevidence.BoundaryRootScalar(ctx.reg, pathEvidenceCoordinateKey(key), value)
			return wrapPathEvidenceCoordinateScalar(scalar), ok
		},
	},
}

func buildPathEvidenceCoordinateFamily(reg *axis.Registry, _ DomainOptions) coordinateFamilyOps {
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
		sealSelectedSkeletonOverlay: func(selected []coordinateKeyPayload, _ *keyspace.KeySpace) (coordinateSkeletonOverlayPlanPayload, bool) {
			plan := make(pathEvidenceCoordinateOverlayPlan, len(selected))
			for index, payload := range selected {
				plan[index] = pathEvidenceCoordinateKey(payload)
			}
			return typedCoordinateSkeletonOverlayPlanPayload[pathEvidenceCoordinateOverlayPlan]{value: plan}, true
		},
		overlaySelectedSkeleton: func(payload coordinateSkeletonOverlayPlanPayload, current, _ coordinateSkeletonPayload, _ []CoordinateScalarFactor, _ *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			if _, ok := payload.(typedCoordinateSkeletonOverlayPlanPayload[pathEvidenceCoordinateOverlayPlan]); !ok {
				return nil, false
			}
			// The four Bottom markers are defaults for unbounded coordinate
			// sub-universes and therefore remain current under finite patching.
			return wrapPathEvidenceCoordinateSkeleton(pathEvidenceCoordinateSkeleton(current)), true
		},
		decompose: func(payload laneFactorPayload, keys *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, error) {
			lane := typedLaneFactorValue[pathevidence.Lane](payload)
			skeleton, entries, ok := pathevidence.DecomposeCoordinates(lane, keys)
			if !ok {
				return nil, nil, fmt.Errorf("invalid path-evidence coordinate decomposition")
			}
			for _, entry := range entries {
				if !pathevidence.CoordinateKeyValid(entry.Key, keys, reg) || !pathevidence.CoordinateScalarValid(entry.Key, entry.Scalar, reg) {
					return nil, nil, fmt.Errorf("%w: invalid path-evidence coordinate payload", ErrInvalidLaneFactor)
				}
			}
			return typedCoordinateSkeletonPayload[pathevidence.CoordinateSkeleton]{value: skeleton}, wrapPathEvidenceCoordinateEntries(entries), nil
		},
		replace: func(_ laneFactorPayload, keys *keyspace.KeySpace, skeleton coordinateSkeletonPayload, entries []coordinateEntry) (laneFactorPayload, error) {
			typedSkeleton := pathEvidenceCoordinateSkeleton(skeleton)
			lane, ok := pathevidence.ComposeCoordinates(typedSkeleton, unwrapPathEvidenceCoordinateEntries(entries), reg, keys)
			if !ok {
				return nil, fmt.Errorf("invalid path-evidence coordinate composition")
			}
			return typedLaneFactorPayload[pathevidence.Lane]{value: lane}, nil
		},

		skeletonBottom: func() coordinateSkeletonPayload {
			return typedCoordinateSkeletonPayload[pathevidence.CoordinateSkeleton]{value: pathevidence.CoordinateBottom()}
		},
		skeletonTop: func() coordinateSkeletonPayload {
			return typedCoordinateSkeletonPayload[pathevidence.CoordinateSkeleton]{value: pathevidence.CoordinateTop()}
		},
		skeletonEqual: func(a, b coordinateSkeletonPayload) bool {
			return pathevidence.CoordinateSkeletonEqual(pathEvidenceCoordinateSkeleton(a), pathEvidenceCoordinateSkeleton(b))
		},
		skeletonLessOrEq: func(a, b coordinateSkeletonPayload) bool {
			return pathevidence.CoordinateSkeletonLessOrEq(pathEvidenceCoordinateSkeleton(a), pathEvidenceCoordinateSkeleton(b))
		},
		skeletonJoin: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapPathEvidenceCoordinateSkeleton(pathevidence.CoordinateSkeletonJoin(pathEvidenceCoordinateSkeleton(a), pathEvidenceCoordinateSkeleton(b)))
		},
		skeletonMeet: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapPathEvidenceCoordinateSkeleton(pathevidence.CoordinateSkeletonMeet(pathEvidenceCoordinateSkeleton(a), pathEvidenceCoordinateSkeleton(b)))
		},
		skeletonWiden: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapPathEvidenceCoordinateSkeleton(pathevidence.CoordinateSkeletonJoin(pathEvidenceCoordinateSkeleton(a), pathEvidenceCoordinateSkeleton(b)))
		},
		skeletonNarrow: func(a, b coordinateSkeletonPayload) coordinateSkeletonPayload {
			return wrapPathEvidenceCoordinateSkeleton(pathevidence.CoordinateSkeletonNarrow(pathEvidenceCoordinateSkeleton(a), pathEvidenceCoordinateSkeleton(b)))
		},
		skeletonHash: func(value coordinateSkeletonPayload) uint64 {
			return pathevidence.CoordinateSkeletonHash(pathEvidenceCoordinateSkeleton(value))
		},
		importSkeleton: func(source coordinateSkeletonPayload, _, _ *keyspace.KeySpace) (coordinateSkeletonPayload, bool) {
			return wrapPathEvidenceCoordinateSkeleton(pathEvidenceCoordinateSkeleton(source)), true
		},

		keyValid: func(key coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return pathevidence.CoordinateKeyValid(pathEvidenceCoordinateKey(key), keys, reg)
		},
		keyEqual: func(a, b coordinateKeyPayload) bool {
			return pathEvidenceCoordinateKey(a) == pathEvidenceCoordinateKey(b)
		},
		keyLess: func(a, b coordinateKeyPayload, keys *keyspace.KeySpace) bool {
			return pathevidence.CoordinateKeyLess(pathEvidenceCoordinateKey(a), pathEvidenceCoordinateKey(b), keys)
		},
		keyHash: func(key coordinateKeyPayload, keys *keyspace.KeySpace) uint64 {
			return pathevidence.CoordinateKeyHash(reg, keys, pathEvidenceCoordinateKey(key))
		},
		importKey: func(source coordinateKeyPayload, from, to *keyspace.KeySpace) (coordinateKeyPayload, bool) {
			key, ok := pathevidence.ImportCoordinateKey(pathEvidenceCoordinateKey(source), from, to, reg)
			if !ok {
				return nil, false
			}
			return typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: key}, true
		},
		formalRekey: coordinateFormalRekeyPolicy{
			kind: coordinateFormalRekeyStructural,
			skeleton: func(source coordinateSkeletonPayload, _ CoordinateFormalRootRekey) (coordinateSkeletonPayload, bool) {
				return wrapPathEvidenceCoordinateSkeleton(pathEvidenceCoordinateSkeleton(source)), true
			},
			key: func(source coordinateKeyPayload, plan CoordinateFormalRootRekey) (coordinateKeyPayload, bool) {
				key, ok := pathevidence.MapCoordinateKeyPaths(pathEvidenceCoordinateKey(source), plan.rekey)
				if !ok {
					return nil, false
				}
				return typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: key}, true
			},
		},
		visitValueDependencies: func(source coordinateKeyPayload, keys *keyspace.KeySpace, visit func(statekey.ValueDependency)) {
			pathevidence.VisitCoordinateValueDependencies(pathEvidenceCoordinateKey(source), keys, visit)
		},

		defaultScalar: func(skeleton coordinateSkeletonPayload, key coordinateKeyPayload) (coordinateScalarPayload, error) {
			return wrapPathEvidenceCoordinateScalar(pathevidence.CoordinateDefault(
				pathEvidenceCoordinateSkeleton(skeleton), pathEvidenceCoordinateKey(key), reg,
			)), nil
		},
		scalarSupport: func(skeleton coordinateSkeletonPayload, key coordinateKeyPayload) CoordinateScalarSupport {
			if pathevidence.CoordinateKeySupported(pathEvidenceCoordinateSkeleton(skeleton), pathEvidenceCoordinateKey(key)) {
				return CoordinateScalarOptional
			}
			return CoordinateScalarForbidden
		},
		scalarValid: func(key coordinateKeyPayload, scalar coordinateScalarPayload) bool {
			return pathevidence.CoordinateScalarValid(pathEvidenceCoordinateKey(key), pathEvidenceCoordinateScalar(scalar), reg)
		},
		scalarEqual: func(a, b coordinateScalarPayload) bool {
			return pathevidence.CoordinateScalarEqual(reg, pathEvidenceCoordinateScalar(a), pathEvidenceCoordinateScalar(b))
		},
		scalarLessOrEq: func(a, b coordinateScalarPayload) bool {
			return pathevidence.CoordinateScalarLessOrEq(reg, pathEvidenceCoordinateScalar(a), pathEvidenceCoordinateScalar(b))
		},
		scalarJoin: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapPathEvidenceCoordinateScalar(pathevidence.CoordinateScalarJoin(reg, pathEvidenceCoordinateScalar(a), pathEvidenceCoordinateScalar(b)))
		},
		scalarMeet: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapPathEvidenceCoordinateScalar(pathevidence.CoordinateScalarMeet(reg, pathEvidenceCoordinateScalar(a), pathEvidenceCoordinateScalar(b)))
		},
		scalarWiden: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapPathEvidenceCoordinateScalar(pathevidence.CoordinateScalarWiden(reg, pathEvidenceCoordinateScalar(a), pathEvidenceCoordinateScalar(b)))
		},
		scalarNarrow: func(a, b coordinateScalarPayload) coordinateScalarPayload {
			return wrapPathEvidenceCoordinateScalar(pathevidence.CoordinateScalarNarrow(reg, pathEvidenceCoordinateScalar(a), pathEvidenceCoordinateScalar(b)))
		},
		scalarHash: func(value coordinateScalarPayload) uint64 {
			return pathevidence.CoordinateScalarHash(reg, pathEvidenceCoordinateScalar(value))
		},
		importScalar: func(source coordinateScalarPayload) (coordinateScalarPayload, bool) {
			scalar := pathEvidenceCoordinateScalar(source)
			if !pathevidence.CoordinateScalarBelongsTo(scalar, reg) {
				return nil, false
			}
			return wrapPathEvidenceCoordinateScalar(scalar), true
		},
		returnIdentity: noCoordinateReturnIdentity(),
		pathEvidence: uniqueCoordinatePathEvidence(
			func(skeleton coordinateSkeletonPayload, entries []coordinateEntry, keys *keyspace.KeySpace, implications []pathevidence.PathPresenceImplication) (coordinateSkeletonPayload, []coordinateEntry, bool) {
				publishedSkeleton, published, ok := pathevidence.ApplyCoordinatePresenceImplications(
					pathEvidenceCoordinateSkeleton(skeleton), unwrapPathEvidenceCoordinateEntries(entries), reg, keys, implications,
				)
				if !ok {
					return nil, nil, false
				}
				return wrapPathEvidenceCoordinateSkeleton(publishedSkeleton), wrapPathEvidenceCoordinateEntries(published), true
			},
			func(skeleton coordinateSkeletonPayload, entries []coordinateEntry, keys *keyspace.KeySpace) (coordinatePathEvidenceCarrier, bool) {
				lane, ok := pathevidence.ComposeCoordinates(
					pathEvidenceCoordinateSkeleton(skeleton), unwrapPathEvidenceCoordinateEntries(entries), reg, keys,
				)
				if !ok {
					return nil, false
				}
				return &pathEvidenceCoordinateCarrier{reg: reg, keys: keys, lane: lane}, true
			},
		),
		pathValues: uniqueCoordinatePathValues(
			func(keys []coordinateKeyPayload, keyspaceOwner *keyspace.KeySpace, seeds []keyspace.Key, roots []symbol.ID) ([]bool, bool) {
				typed := make([]pathevidence.CoordinateKey, len(keys))
				for index, value := range keys {
					typed[index] = pathEvidenceCoordinateKey(value)
				}
				return pathevidence.CoordinatePathMutationClosure(typed, keyspaceOwner, seeds, roots)
			},
			func(keys *keyspace.KeySpace, union []coordinateKeyPayload, seeds []coordinateDependencySeedPayload) (coordinateDependencyPlanPayload, bool) {
				typedUnion := make([]pathevidence.CoordinateKey, len(union))
				for index, value := range union {
					typedUnion[index] = pathEvidenceCoordinateKey(value)
				}
				typedSeeds := make([]pathevidence.CoordinateDependencySeed, len(seeds))
				for index, seed := range seeds {
					readCoordinates := make([]pathevidence.CoordinateKey, len(seed.readCoordinates))
					for readIndex, value := range seed.readCoordinates {
						readCoordinates[readIndex] = pathEvidenceCoordinateKey(value)
					}
					add := make([]pathevidence.CoordinateKey, len(seed.add))
					for addIndex, value := range seed.add {
						add[addIndex] = pathEvidenceCoordinateKey(value)
					}
					equalities := make([]pathevidence.CoordinateDependencyEquality, len(seed.transientEqualities))
					for equalityIndex, equality := range seed.transientEqualities {
						equalities[equalityIndex] = pathevidence.CoordinateDependencyEquality{Left: equality.left, Right: equality.right}
					}
					typedSeeds[index] = pathevidence.CoordinateDependencySeed{
						ID: pathevidence.CoordinateDependencyID(seed.id), ReadPaths: seed.readPaths, ResolvePaths: seed.resolvePaths,
						WritePaths: seed.writePaths, DescendantMutationRoots: seed.mutationRoots,
						SubtreeMutationRoots: seed.subtreeRoots,
						StableRootMutations:  seed.stableRootMutations,
						FormalStableRoots:    seed.formalStableRoots,
						TransientEqualities:  equalities, ReadCoordinates: readCoordinates, AddCoordinates: add,
					}
				}
				plan, ok := pathevidence.PlanCoordinateDependencies(reg, keys, typedUnion, typedSeeds)
				if !ok {
					return coordinateDependencyPlanPayload{}, false
				}
				out := coordinateDependencyPlanPayload{
					byID:    make(map[uint64]coordinateDependencyPayload),
					order:   make([]uint64, 0, len(typedSeeds)),
					depends: make(map[coordinateDependencyEdgePayload]struct{}),
					feeds:   make(map[coordinateDependencyEdgePayload]struct{}),
				}
				for _, value := range plan.Coordinates() {
					out.coordinates = append(out.coordinates, typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: value})
				}
				for _, id := range plan.IDs() {
					dep, present := plan.Dependency(id)
					if !present {
						return coordinateDependencyPlanPayload{}, false
					}
					wrapped := coordinateDependencyPayload{id: uint64(id)}
					for _, value := range dep.CoordinateReads {
						wrapped.reads = append(wrapped.reads, typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: value})
					}
					for _, value := range dep.CoordinateWrites {
						wrapped.writes = append(wrapped.writes, typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: value})
					}
					wrapLocation := func(value pathevidence.CoordinateDependencyLocation) coordinateDependencyLocationPayload {
						return coordinateDependencyLocationPayload{root: value.Root, path: value.Path}
					}
					for _, value := range dep.LocationReads {
						wrapped.locationReads = append(wrapped.locationReads, wrapLocation(value))
					}
					for _, value := range dep.LocationWrites {
						wrapped.locationWrites = append(wrapped.locationWrites, wrapLocation(value))
					}
					for _, value := range dep.MutationRegions {
						wrapped.mutationRegions = append(wrapped.mutationRegions, wrapLocation(value))
					}
					out.order = append(out.order, uint64(id))
					out.byID[uint64(id)] = wrapped
				}
				for _, writer := range plan.IDs() {
					for _, reader := range plan.IDs() {
						if plan.Depends(writer, reader) {
							out.depends[coordinateDependencyEdgePayload{writer: uint64(writer), reader: uint64(reader)}] = struct{}{}
						}
						if plan.Feeds(writer, reader) {
							out.feeds[coordinateDependencyEdgePayload{writer: uint64(writer), reader: uint64(reader)}] = struct{}{}
						}
					}
				}
				return out, true
			},
			func(source coordinateKeyPayload) []keyspace.Key {
				out := make([]keyspace.Key, 0, 3)
				pathevidence.ExpandCoordinateClosure(
					pathEvidenceCoordinateKey(source),
					func(paths ...keyspace.Key) bool {
						out = append(out, paths...)
						return true
					},
					nil,
				)
				return out
			},
		),
		rootAssignment: noCoordinateRootAssignment(),
		pathMutation:   noCoordinatePathMutation(),
		objectMutation: noCoordinateObjectMutation(),
	})
}

// pathEvidenceCoordinateCarrier is the one registered storage adapter for the
// canonical path-evidence algebra. It contains no trigger, target, barrier or
// scheduling policy; all semantic decisions remain in factapply.
type pathEvidenceCoordinateCarrier struct {
	reg  *axis.Registry
	keys *keyspace.KeySpace
	lane pathevidence.Lane
}

func (c *pathEvidenceCoordinateCarrier) Clone() coordinatePathEvidenceCarrier {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}

func (c *pathEvidenceCoordinateCarrier) MakeUnreachable() {
	if c != nil && c.reg != nil {
		c.lane = pathevidence.Domain(c.reg).Bottom()
	}
}

func (c *pathEvidenceCoordinateCarrier) validPath(path keyspace.Key) bool {
	if c == nil || c.keys == nil || !c.keys.Valid() || path.Kind == keyspace.KindInvalid {
		return false
	}
	_, ok := c.keys.SegmentsView(path)
	return ok
}

func (c *pathEvidenceCoordinateCarrier) SnapshotImplications() (pathevidence.PathPresenceImplicationsSnapshot, bool) {
	if c.keys == nil || !c.keys.Valid() {
		return pathevidence.PathPresenceImplicationsSnapshot{}, false
	}
	return c.lane.PathPresenceImplicationsSnapshot(c.keys), true
}

func (c *pathEvidenceCoordinateCarrier) HasImplication(value pathevidence.PathPresenceImplication) bool {
	return c != nil && c.lane.HasPathPresenceImplication(value)
}

func (c *pathEvidenceCoordinateCarrier) AddImplication(value pathevidence.PathPresenceImplication) (bool, bool) {
	if c.keys == nil || !c.keys.Valid() {
		return false, false
	}
	canonical, ok := pathevidence.CanonicalPathPresenceImplications(c.reg, c.keys, []pathevidence.PathPresenceImplication{value})
	if !ok || len(canonical) != 1 {
		return false, false
	}
	next, changed := c.lane.AddPathPresenceImplication(canonical[0])
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) ReadPath(path keyspace.Key) (product.Value, bool) {
	if !c.validPath(path) {
		return product.Value{}, false
	}
	return c.lane.ReadPathKey(c.reg, path), true
}

func (c *pathEvidenceCoordinateCarrier) WritePath(path keyspace.Key, value product.Value) (bool, bool) {
	if !c.validPath(path) || !product.BelongsToRegistry(c.reg, value) {
		return false, false
	}
	next, changed := c.lane.WritePathKey(c.reg, path, value)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) ReadStaticMember(path keyspace.Key) (product.Value, bool) {
	if !c.validPath(path) {
		return product.Value{}, false
	}
	return c.lane.ReadPathStaticMember(path)
}

func (c *pathEvidenceCoordinateCarrier) WriteStaticMember(path keyspace.Key, value product.Value) (bool, bool) {
	if !c.validPath(path) || !product.BelongsToRegistry(c.reg, value) {
		return false, false
	}
	next, changed := c.lane.WritePathStaticMember(path, value)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) HasProof(proof pathevidence.BranchProof) bool {
	return c.lane.HasBranchProof(proof)
}

func (c *pathEvidenceCoordinateCarrier) AddProof(proof pathevidence.BranchProof) (bool, bool) {
	next, changed := c.lane.AddBranchProof(proof)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) CloseProofsAcrossKnownEqualities(keys *keyspace.KeySpace) (bool, bool) {
	if c == nil || keys == nil || !keys.Valid() {
		return false, false
	}
	next, changed := c.lane.CloseProofsAcrossKnownEqualities(keys)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) CloseProofsAcrossTransientEquality(left, right keyspace.Key) (bool, bool) {
	if c == nil || c.keys == nil || !c.keys.Valid() {
		return false, false
	}
	next, changed := c.lane.CloseProofsAcrossTransientEquality(c.keys, left, right)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) CloseRefinementsAcrossTransientEquality(
	reg *axis.Registry, left, right keyspace.Key, memberSafe bool, allow func(keyspace.Key) bool,
) (bool, bool) {
	if c == nil || reg == nil || allow == nil {
		return false, false
	}
	next, changed := c.lane.CloseRefinementsAcrossTransientEquality(reg, c.keys, left, right, memberSafe, allow)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) EquivalentKeys(path keyspace.Key) ([]keyspace.Key, bool) {
	if !c.validPath(path) {
		return nil, false
	}
	return c.lane.EquivalentKeyspaceKeys(c.keys, path), true
}

func (c *pathEvidenceCoordinateCarrier) HasEquivalentKey(left, right keyspace.Key) (bool, bool) {
	if !c.validPath(left) || !c.validPath(right) {
		return false, false
	}
	return c.lane.HasEquivalentKeyspaceKey(c.keys, left, right), true
}

func (c *pathEvidenceCoordinateCarrier) EqualityQuotient() (pathevidence.EqualityQuotient, bool) {
	if c == nil || c.keys == nil || !c.keys.Valid() {
		return pathevidence.EqualityQuotient{}, false
	}
	return c.lane.SealEqualityQuotient(c.keys)
}

func (c *pathEvidenceCoordinateCarrier) DescendantPrefixes(path pathdom.PathKey) (pathevidence.PathKeyDescendantInvalidationPrefixes, bool) {
	return c.lane.PathKeyDescendantInvalidationPrefixes(c.keys, path)
}

func (c *pathEvidenceCoordinateCarrier) SubtreePrefixes(path pathdom.PathKey) ([]pathdom.PathKey, bool) {
	return c.lane.PathKeySubtreeInvalidationPrefixes(c.keys, path)
}

func (c *pathEvidenceCoordinateCarrier) InvalidateSubtreePrefixes(prefixes []pathdom.PathKey) (bool, bool) {
	if c.keys == nil || !c.keys.Valid() {
		return false, false
	}
	next, changed := c.lane.InvalidatePathKeySubtreePrefixesChanged(c.keys, prefixes)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) ApplyStableRootMutation(mutation StableRootPathEvidenceMutation) (bool, bool) {
	if c == nil || c.reg == nil || mutation.target == 0 {
		return false, false
	}
	next := applyStableRootPathEvidenceMutation(c.lane, mutation)
	changed := !pathevidence.Domain(c.reg).Equal(c.lane, next)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) InvalidatePrefixes(prefixes pathevidence.PathKeyDescendantInvalidationPrefixes) (bool, bool) {
	if c.keys == nil || !c.keys.Valid() {
		return false, false
	}
	next, changed := c.lane.InvalidatePathKeyDescendantPrefixesChanged(c.keys, prefixes)
	if changed {
		c.lane = next
	}
	return changed, true
}

func (c *pathEvidenceCoordinateCarrier) Freeze() (coordinateSkeletonPayload, []coordinateEntry, bool) {
	skeleton, entries, ok := pathevidence.DecomposeCoordinates(c.lane, c.keys)
	if !ok {
		return nil, nil, false
	}
	return wrapPathEvidenceCoordinateSkeleton(skeleton), wrapPathEvidenceCoordinateEntries(entries), true
}

func wrapPathEvidenceCoordinateEntries(entries []pathevidence.CoordinateEntry) []coordinateEntry {
	out := make([]coordinateEntry, len(entries))
	for index, entry := range entries {
		out[index] = coordinateEntry{
			key:    typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: entry.Key},
			scalar: typedCoordinateScalarPayload[pathevidence.CoordinateScalar]{value: entry.Scalar},
		}
	}
	return out
}

func unwrapPathEvidenceCoordinateEntries(entries []coordinateEntry) []pathevidence.CoordinateEntry {
	out := make([]pathevidence.CoordinateEntry, len(entries))
	for index, entry := range entries {
		out[index] = pathevidence.CoordinateEntry{
			Key: pathEvidenceCoordinateKey(entry.key), Scalar: pathEvidenceCoordinateScalar(entry.scalar),
		}
	}
	return out
}

func pathEvidenceCoordinateSkeleton(payload coordinateSkeletonPayload) pathevidence.CoordinateSkeleton {
	typed, ok := payload.(typedCoordinateSkeletonPayload[pathevidence.CoordinateSkeleton])
	if !ok {
		panic("state: path-evidence coordinate skeleton mismatch")
	}
	return typed.value
}

func pathEvidenceCoordinateKey(payload coordinateKeyPayload) pathevidence.CoordinateKey {
	typed, ok := payload.(typedCoordinateKeyPayload[pathevidence.CoordinateKey])
	if !ok {
		panic("state: path-evidence coordinate key mismatch")
	}
	return typed.value
}

func pathEvidenceCoordinateScalar(payload coordinateScalarPayload) pathevidence.CoordinateScalar {
	typed, ok := payload.(typedCoordinateScalarPayload[pathevidence.CoordinateScalar])
	if !ok {
		panic("state: path-evidence coordinate scalar mismatch")
	}
	return typed.value
}

func wrapPathEvidenceCoordinateSkeleton(value pathevidence.CoordinateSkeleton) coordinateSkeletonPayload {
	return typedCoordinateSkeletonPayload[pathevidence.CoordinateSkeleton]{value: value}
}

func wrapPathEvidenceCoordinateScalar(value pathevidence.CoordinateScalar) coordinateScalarPayload {
	return typedCoordinateScalarPayload[pathevidence.CoordinateScalar]{value: value}
}

func applyPathEvidenceCoordinateSkeleton(_ *boundaryApplyContext, destination, fragment coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
	return wrapPathEvidenceCoordinateSkeleton(pathevidence.ApplyCoordinateSkeletonBoundary(
		pathEvidenceCoordinateSkeleton(destination), pathEvidenceCoordinateSkeleton(fragment),
	)), true
}

func applyPathEvidenceCoordinateScalar(key coordinateKeyPayload, destination, fragment coordinateScalarPayload, affected bool) (coordinateScalarPayload, bool) {
	return wrapPathEvidenceCoordinateScalar(pathevidence.ApplyCoordinateScalarBoundary(
		pathEvidenceCoordinateKey(key), pathEvidenceCoordinateScalar(destination), pathEvidenceCoordinateScalar(fragment), func(keyspace.Key) bool { return affected },
	)), true
}
