package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneValues LaneID = "values"

var valuesLaneSpec = laneSpec{
	id:                       LaneValues,
	slotFactored:             true,
	keySpaceMode:             laneKeySpaceFree,
	valueDependencies:        independentValueDependencies(),
	identitySupport:          enumeratedIdentitySupport(visitValuesLaneIdentities, func(s State) valueLane { return s.values }, IdentityImageEmbeddedValue),
	numericConsistency:       numericConsistencyIndependent(),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementLane[valueLane](false, true, true, applyPathReplacementValuesLane), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(true, true, true),
	dynamicRead:              dynamicReadIndependent(),
	fingerprint:              fingerprintValues,
	boundary:                 boundaryLaneOps{project: projectValuesBoundary, rebase: rebaseValuesBoundary, postRebase: postRebaseBoundaryNoop, equal: equalValuesBoundary},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := valueLaneDomain(reg)
		return stateLaneWithBoundary(domain,
			func(s State) valueLane {
				return s.values
			},
			func(out *State, values valueLane) {
				out.values = values
			},
			typedLaneFactorRepresentation[valueLane]{equal: domain.Equal},
			typedBoundaryFactorOps[valueLane]{
				apply: applyValuesBoundaryLane, roots: boundaryRootsSlotValues(applyValuesBoundaryRoots),
				project: projectValuesBoundaryFactor, rebase: rebaseValuesBoundaryFactor,
				postRebase: boundaryPostRebaseUnchanged[valueLane], reachability: emitValuesReachability,
			},
		)
	},
}
