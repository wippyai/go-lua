package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneStoreRelations LaneID = "store-relations"

var storeRelationsLaneSpec = laneSpec{
	id:                       LaneStoreRelations,
	keySpaceMode:             laneKeySpaceFree,
	formalRekey:              formalRekeyStructural(),
	valueDependencies:        independentValueDependencies(),
	identitySupport:          independentIdentitySupport(),
	numericConsistency:       numericConsistencyIndependent(),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, false, false),
	dynamicRead:              dynamicReadIndependent(),
	fingerprint:              fingerprintStoreRelations,
	boundary:                 boundaryLaneOps{project: projectStoreRelationsBoundary, rebase: rebaseStoreRelationsBoundary, postRebase: postRebaseBoundaryNoop, equal: equalStoreRelationsBoundary},
	markReachable: func(s State) State {
		s.storeRelations = s.storeRelations.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := storeRelationDomain()
		return stateLaneWithBoundary(domain,
			func(s State) storeRelationLane { return s.storeRelations },
			func(out *State, lane storeRelationLane) { out.storeRelations = lane },
			typedLaneFactorRepresentation[storeRelationLane]{equal: domain.Equal},
			typedBoundaryFactorOps[storeRelationLane]{apply: applyStoreRelationsBoundaryLane, roots: boundaryRootsReachable(func(lane storeRelationLane) storeRelationLane { return lane.reachable() }), project: projectStoreRelationsBoundaryFactor, rebase: rebaseStoreRelationsBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[storeRelationLane], reachability: emitStoreRelationsReachability},
		)
	},
}

func emitStoreRelationsReachability(program *boundaryReachabilityProgramBuilder, lane storeRelationLane) {
	if lane.bottom {
		return
	}
	for relation := range lane.values {
		program.pathCone(false, program.addStateKey(relation.Source), program.addStateKey(relation.Into))
	}
}
