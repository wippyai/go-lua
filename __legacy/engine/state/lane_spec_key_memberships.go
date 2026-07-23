package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

const LaneKeyMemberships LaneID = "key-memberships"

var keyMembershipsLaneSpec = laneSpec{
	id:                 LaneKeyMemberships,
	keySpaceMode:       laneKeySpaceOwned,
	formalRekey:        formalRekeyStructural(),
	valueDependencies:  independentValueDependencies(),
	identitySupport:    independentIdentitySupport(),
	numericConsistency: numericConsistencyIndependent(),
	semanticLaws: []laneSemanticLaw{pathSubtreeMutationLane(
		func(s State) keyMembershipLane { return s.keyMemberships },
		func(out *State, lane keyMembershipLane) { out.keyMemberships = lane },
		func(lane keyMembershipLane, keys *keyspace.KeySpace, _ []pathdom.PathKey, path pathdom.PathKey) (keyMembershipLane, bool, bool) {
			return lane.clearPathKeySubtree(keys, path)
		},
	), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientLane(
		func(s State) keyMembershipLane { return s.keyMemberships },
		func(out *State, lane keyMembershipLane) { out.keyMemberships = lane },
		applyPathEqualityKeyMemberships,
	), genericForKeyMembershipBinding(), pathReplacementLane[keyMembershipLane](false, true, true, applyPathReplacementMembershipLane), effectFactorLaneWithObserver(effectFactorDynamicIndexMembership,
		func(s State) keyMembershipLane { return s.keyMemberships },
		func(out *State, lane keyMembershipLane) { out.keyMemberships = lane },
		applyKeyMembershipEffectFactor,
		observeDynamicIndexMembershipEvidence,
	), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment: withRootAssignmentDynamicSourceLaw(rootAssignmentCompletionLane(rootAssignmentCompletionFreshEmptyDependencies(), false, true, true,
		func(s State) keyMembershipLane { return s.keyMemberships },
		func(out *State, lane keyMembershipLane) { out.keyMemberships = lane },
		applyRootAssignmentKeyMemberships,
	)),
	dynamicRead: dynamicReadMemberships(),
	boundary:    boundaryLaneOps{project: projectKeyMembershipsBoundary, rebase: rebaseKeyMembershipsBoundary, postRebase: postRebaseBoundaryNoop, equal: equalKeyMembershipsBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.keyMemberships.rekey(from, to)
		if !ok {
			return s, false
		}
		s.keyMemberships = lane
		return s, true
	},
	fingerprint: fingerprintKeyMemberships,
	markReachable: func(s State) State {
		s.keyMemberships = s.keyMemberships.reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := keyMembershipDomain()
		return stateLaneWithBoundary(domain,
			func(s State) keyMembershipLane { return s.keyMemberships },
			func(out *State, lane keyMembershipLane) { out.keyMemberships = lane },
			typedLaneFactorRepresentation[keyMembershipLane]{equal: domain.Equal},
			typedBoundaryFactorOps[keyMembershipLane]{
				apply: applyKeyMembershipsBoundaryLane, roots: boundaryRootsReachable(func(lane keyMembershipLane) keyMembershipLane { return lane.reachable() }),
				project: projectKeyMembershipsBoundaryFactor, rebase: rebaseKeyMembershipsBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[keyMembershipLane],
				reachability: emitKeyMembershipLaneReachability,
			},
		)
	},
}

func emitKeyMembershipLaneReachability(program *boundaryReachabilityProgramBuilder, lane keyMembershipLane) {
	if lane.bottom {
		return
	}
	for membership := range lane.path {
		if membershipBoundaryRegistered(program.keys, membership) {
			program.pathCone(false, program.addStateKey(membership.Key), program.addStateKey(membership.Table))
		}
	}
	for membership := range lane.dynamic {
		if membershipBoundaryRegistered(program.keys, membership) {
			program.pathCone(true, membership.Container, program.addStateKey(membership.Table))
		}
	}
	for membership := range lane.dynamicAll {
		if membershipBoundaryRegistered(program.keys, membership) {
			program.pathCone(true, membership.Container, program.addStateKey(membership.Table))
		}
	}
	for origin := range lane.valueOrigins {
		if valueOriginBoundaryRegistered(program.keys, origin) {
			program.pathCone(false, program.addStateKey(origin.Value), origin.Container)
		}
	}
	for origin := range lane.readOrigins {
		if readOriginBoundaryRegistered(program.keys, origin) {
			program.pathCone(false, program.addStateKey(origin.Value), origin.Container, program.addStateKey(origin.Key))
		}
	}
	for restore := range lane.pendingRestores {
		if restoreBoundaryRegistered(program.keys, restore) {
			program.pathCone(false, restore.Container, program.addStateKey(restore.Table), program.addStateKey(restore.Key))
		}
	}
}
