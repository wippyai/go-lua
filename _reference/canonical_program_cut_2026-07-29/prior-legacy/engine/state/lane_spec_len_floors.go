package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

const LaneLenFloors LaneID = "len-floors"

var lenFloorsLaneSpec = laneSpec{
	id:                 LaneLenFloors,
	keySpaceMode:       laneKeySpaceOwned,
	valueDependencies:  independentValueDependencies(),
	identitySupport:    independentIdentitySupport(),
	numericConsistency: numericConsistencyContributor(contributeLenFloors),
	coordinateFamilies: []coordinateFamilySpec{lenFloorCoordinateFamilySpec},
	semanticLaws: []laneSemanticLaw{pathSubtreeMutationLane(
		func(s State) lenFloorLane { return s.lenFloors },
		setStateLenFloors,
		func(lane lenFloorLane, keys *keyspace.KeySpace, prefixes []pathdom.PathKey, _ pathdom.PathKey) (lenFloorLane, bool, bool) {
			if keys == nil || !keys.Valid() {
				return lane, false, false
			}
			next, changed := lane.clearPathKeySubtrees(keys, prefixes)
			return next, changed, true
		},
	), pathDescendantMutationLane(
		func(s State) lenFloorLane { return s.lenFloors },
		setStateLenFloors,
		func(lane lenFloorLane, keys *keyspace.KeySpace, prefixes pathevidence.PathKeyDescendantInvalidationPrefixes, _ pathdom.PathKey) (lenFloorLane, bool, bool) {
			if keys == nil || !keys.Valid() {
				return lane, false, false
			}
			next, changed := lane.clearPathKeyDescendantMutation(keys, prefixes)
			return next, changed, true
		},
	), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementLane[lenFloorLane](false, true, true, applyPathReplacementLenFloorLane), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(true, true, true),
	dynamicRead:              dynamicReadIndependent(),
	boundary:                 boundaryLaneOps{project: projectLenFloorsBoundary, rebase: rebaseLenFloorsBoundary, postRebase: postRebaseBoundaryNoop, equal: equalLenFloorsBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.lenFloors.rekey(from, to)
		if !ok {
			return s, false
		}
		setStateLenFloors(&s, lane)
		return s, true
	},
	fingerprint: fingerprintLenFloors,
	markReachable: func(s State) State {
		if s.lenFloors.lane.Bottom() {
			setStateLenFloors(&s, s.lenFloors.reachable())
		}
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := lenFloorMapDomain()
		return stateLaneWithBoundary(domain,
			func(s State) lenFloorLane { return s.lenFloors },
			setStateLenFloors,
			typedLaneFactorRepresentation[lenFloorLane]{equal: domain.Equal},
			typedBoundaryFactorOps[lenFloorLane]{apply: applyLenFloorsBoundaryLane, roots: boundaryRootsReachable(func(lane lenFloorLane) lenFloorLane { return lane.reachable() }), project: projectLenFloorsBoundaryFactor, rebase: rebaseLenFloorsBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[lenFloorLane], reachability: emitLenFloorsReachability},
		)
	},
}

func emitLenFloorsReachability(program *boundaryReachabilityProgramBuilder, lane lenFloorLane) {
	if lane.lane.Bottom() {
		return
	}
	for path := range lane.lane.Values() {
		program.pathCone(false, path)
	}
}
