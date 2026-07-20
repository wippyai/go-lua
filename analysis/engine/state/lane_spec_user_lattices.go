package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

const LaneUserLattices LaneID = "user-lattices"

var userLatticesLaneSpec = laneSpec{
	dynamicRead:        dynamicReadIndependent(),
	id:                 LaneUserLattices,
	keySpaceMode:       laneKeySpaceOwned,
	formalRekey:        formalRekeyStructural(),
	valueDependencies:  independentValueDependencies(),
	identitySupport:    independentIdentitySupport(),
	numericConsistency: numericConsistencyIndependent(),
	semanticLaws: []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementLane[userLatticeLane](true, true, true, applyPathReplacementUserLane), effectFactorIndependent(), callBoundaryLane[userLatticeLane](
		func(s State) userLatticeLane { return s.userLattices },
		func(out *State, lane userLatticeLane) { out.userLattices = lane },
		applyUserLatticeCallBoundary,
	)},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment: withRootAssignmentScalarLaw(
		rootAssignmentUnchanged(true, true, true),
		func(s State) userLatticeLane { return s.userLattices },
		func(out *State, lane userLatticeLane) { out.userLattices = lane },
		applyRootAssignmentUserScalar,
	),
	boundary: boundaryLaneOps{project: projectUserLatticesBoundary, rebase: rebaseUserLatticesBoundary, postRebase: postRebaseBoundaryNoop, equal: equalUserLatticesBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.userLattices.rekey(from, to)
		if !ok {
			return s, false
		}
		s.userLattices = lane
		return s, true
	},
	fingerprint: fingerprintUserLattices,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		rt := userlattice.RuntimeFor(reg)
		domain := userLatticeDomain(rt)
		return stateLaneWithBoundary(domain,
			func(s State) userLatticeLane { return s.userLattices },
			func(out *State, lane userLatticeLane) { out.userLattices = lane },
			typedLaneFactorRepresentation[userLatticeLane]{equal: domain.Equal},
			typedBoundaryFactorOps[userLatticeLane]{apply: applyUserLatticesBoundaryLane, roots: boundaryRootsUnchanged[userLatticeLane](), project: projectUserLatticesBoundaryFactor, rebase: rebaseUserLatticesBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[userLatticeLane], reachability: emitUserLatticesReachability},
		)
	},
}

func emitUserLatticesReachability(program *boundaryReachabilityProgramBuilder, lane userLatticeLane) {
	if lane.top {
		return
	}
	for userKey := range lane.values {
		program.pathCone(false, userKey.path)
	}
}
