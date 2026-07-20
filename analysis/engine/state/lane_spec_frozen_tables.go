package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneFrozenTables LaneID = "frozen-tables"

var frozenTablesLaneSpec = laneSpec{
	id:                       LaneFrozenTables,
	keySpaceMode:             laneKeySpaceFree,
	formalRekey:              formalRekeyIndependent(),
	valueDependencies:        independentValueDependencies(),
	identitySupport:          enumeratedIdentitySupport(visitFrozenTablesLaneIdentities, func(s State) frozenTableLane { return s.frozenTables }, IdentityImageMustSet),
	numericConsistency:       numericConsistencyIndependent(),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, false, false),
	dynamicRead:              dynamicReadIndependent(),
	fingerprint:              fingerprintFrozenTables,
	boundary:                 boundaryLaneOps{project: projectFrozenBoundary, rebase: rebaseFrozenBoundary, postRebase: postRebaseBoundaryNoop, equal: equalFrozenBoundary},
	markReachable: func(s State) State {
		s.frozenTables = s.frozenTables.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := frozenTableDomain()
		return stateLaneWithBoundary(domain,
			func(s State) frozenTableLane { return s.frozenTables },
			func(out *State, lane frozenTableLane) { out.frozenTables = lane },
			typedLaneFactorRepresentation[frozenTableLane]{equal: domain.Equal},
			typedBoundaryFactorOps[frozenTableLane]{apply: applyFrozenBoundaryLane, roots: boundaryRootsReachable(func(lane frozenTableLane) frozenTableLane { return lane.reachable() }), project: projectFrozenBoundaryFactor, rebase: rebaseFrozenBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[frozenTableLane], reachability: emitFrozenTablesReachability},
		)
	},
}

func emitFrozenTablesReachability(program *boundaryReachabilityProgramBuilder, lane frozenTableLane) {
	for term := range lane.values {
		if program.allIdentities() {
			program.addIdentityTerm(term)
		}
	}
}
