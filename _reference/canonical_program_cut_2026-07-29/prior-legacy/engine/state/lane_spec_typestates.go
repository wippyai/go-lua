package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

const LaneTypestates LaneID = "typestates"

var typestatesLaneSpec = laneSpec{
	id:                 LaneTypestates,
	keySpaceMode:       laneKeySpaceFree,
	formalRekey:        formalRekeyStructural(),
	valueDependencies:  independentValueDependencies(),
	identitySupport:    independentIdentitySupport(),
	numericConsistency: numericConsistencyIndependent(),
	semanticLaws: []laneSemanticLaw{
		pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(),
		pathEqualityQuotientLane(
			func(s State) typestate.Store { return s.typestates },
			func(out *State, store typestate.Store) { out.typestates = store },
			applyPathEqualityTypestates,
		),
		genericForBindingIndependent(),
		pathReplacementLane[typestate.Store](false, true, true, applyPathReplacementTypestateLane),
		effectFactorIndependent(),
		callBoundaryIndependent(),
	},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, true, true),
	dynamicRead:              dynamicReadIndependent(),
	fingerprint:              fingerprintTypestates,
	boundary:                 boundaryLaneOps{project: projectTypestatesBoundary, rebase: rebaseTypestatesBoundary, postRebase: postRebaseBoundaryNoop, equal: equalTypestatesBoundary},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := typestate.Domain
		return stateLaneWithBoundary(domain,
			func(s State) typestate.Store { return s.typestates },
			func(out *State, store typestate.Store) { out.typestates = store },
			typedLaneFactorRepresentation[typestate.Store]{equal: domain.Equal},
			typedBoundaryFactorOps[typestate.Store]{apply: applyTypestatesBoundaryLane, roots: boundaryRootsUnchanged[typestate.Store](), project: projectTypestatesBoundaryFactor, rebase: rebaseTypestatesBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[typestate.Store], reachability: emitTypestatesReachability},
		)
	},
}

func emitTypestatesReachability(program *boundaryReachabilityProgramBuilder, lane typestate.Store) {
	for _, resource := range lane.Resources() {
		if path, ok := program.keys.FromStateKey(pathdom.PathKey(resource.ID.String())); ok {
			program.pathCone(false, path)
		}
	}
}
