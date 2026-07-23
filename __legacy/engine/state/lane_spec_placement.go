package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

const LanePlacement LaneID = "placement"

var placementLaneSpec = laneSpec{
	id:                       LanePlacement,
	keySpaceMode:             laneKeySpaceFree,
	valueDependencies:        independentValueDependencies(),
	identitySupport:          enumeratedIdentitySupport(visitPlacementLaneIdentities, func(s State) placementLane { return s.placement }, IdentityImagePointwiseMap),
	numericConsistency:       numericConsistencyIndependent(),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, true, true),
	dynamicRead:              dynamicReadIndependent(),
	coordinateFamilies:       []coordinateFamilySpec{placementCoordinateFamilySpec},
	fingerprint:              fingerprintPlacement,
	boundary:                 boundaryLaneOps{project: projectPlacementBoundary, rebase: rebasePlacementBoundary, postRebase: postRebaseBoundaryNoop, equal: equalPlacementBoundary},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := placementMapDomain()
		laneDomain := wrapDomain(domain,
			func(placements map[identity.Term]placement.Value) placementLane {
				return placementLaneFromMap(domain, placements)
			},
			func(lane placementLane) map[identity.Term]placement.Value { return lane.asMap(domain) },
		)
		return stateLaneWithBoundary(laneDomain,
			func(s State) placementLane { return s.placement },
			func(out *State, lane placementLane) { out.placement = lane },
			typedLaneFactorRepresentation[placementLane]{equal: laneDomain.Equal},
			typedBoundaryFactorOps[placementLane]{apply: applyPlacementBoundaryLane, roots: boundaryRootsUnchanged[placementLane](), project: projectPlacementBoundaryFactor, rebase: rebasePlacementBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[placementLane], reachability: emitPlacementReachability},
		)
	},
}

func emitPlacementReachability(program *boundaryReachabilityProgramBuilder, lane placementLane) {
	for term := range lane.values {
		if program.allIdentities() {
			program.addIdentityTerm(term)
		}
	}
}

func placementMapDomain() lattice.Lattice[map[identity.Term]placement.Value] {
	return lift.Map[identity.Term, placement.Value](placement.Lattice())
}
