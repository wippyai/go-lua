package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

const LaneDynamicIndex LaneID = "dynamic-index"

var dynamicIndexLaneSpec = laneSpec{
	id:                 LaneDynamicIndex,
	keySpaceMode:       laneKeySpaceOwned,
	formalRekey:        formalRekeyStructural(),
	valueDependencies:  independentValueDependencies(),
	identitySupport:    enumeratedIdentitySupport(visitDynamicIndexLaneIdentities, func(s State) dynamicIndexLane { return s.dynamicIndex }, IdentityImageEmbeddedValue),
	numericConsistency: numericConsistencyIndependent(),
	semanticLaws: []laneSemanticLaw{pathSubtreeMutationLane(
		func(s State) dynamicIndexLane { return s.dynamicIndex },
		func(out *State, lane dynamicIndexLane) { out.dynamicIndex = lane },
		func(lane dynamicIndexLane, keys *keyspace.KeySpace, _ []pathdom.PathKey, path pathdom.PathKey) (dynamicIndexLane, bool, bool) {
			return lane.clearPathKeySubtree(keys, path)
		},
	), pathDescendantMutationLane(
		func(s State) dynamicIndexLane { return s.dynamicIndex },
		func(out *State, lane dynamicIndexLane) { out.dynamicIndex = lane },
		func(lane dynamicIndexLane, keys *keyspace.KeySpace, _ pathevidence.PathKeyDescendantInvalidationPrefixes, path pathdom.PathKey) (dynamicIndexLane, bool, bool) {
			return lane.clearPathKeyDescendants(keys, path)
		},
	), pathResolutionParticipant(), pathEqualityQuotientLane(
		func(s State) dynamicIndexLane { return s.dynamicIndex },
		func(out *State, lane dynamicIndexLane) { out.dynamicIndex = lane },
		applyPathEqualityDynamicIndex,
	), genericForDynamicIndexBinding(), pathReplacementLane[dynamicIndexLane](false, true, true, applyPathReplacementDynamicLane), effectFactorLane(effectFactorDynamicIndexMembership,
		func(s State) dynamicIndexLane { return s.dynamicIndex },
		func(out *State, lane dynamicIndexLane) { out.dynamicIndex = lane },
		applyDynamicIndexEffectFactor,
	), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           withRootAssignmentDynamicSourceFactsInput(rootAssignmentUnchanged(false, true, true)),
	dynamicRead:              dynamicReadFacts(),
	boundary:                 boundaryLaneOps{project: projectDynamicIndexBoundary, rebase: rebaseDynamicIndexBoundary, postRebase: postRebaseBoundaryNoop, equal: equalDynamicIndexBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.dynamicIndex.rekey(from, to)
		if !ok {
			return s, false
		}
		s.dynamicIndex = lane
		return s, true
	},
	fingerprint: fingerprintDynamicIndex,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := dynamicindex.MapDomain(reg)
		laneDomain := wrapDomain(domain,
			func(facts map[dynamicindex.Key]dynamicindex.Fact) dynamicIndexLane {
				return dynamicIndexLaneFromMap(domain, facts)
			},
			func(lane dynamicIndexLane) map[dynamicindex.Key]dynamicindex.Fact { return lane.asMap(domain) },
		)
		return stateLaneWithBoundary(laneDomain,
			func(s State) dynamicIndexLane { return s.dynamicIndex },
			func(out *State, lane dynamicIndexLane) { out.dynamicIndex = lane },
			typedLaneFactorRepresentation[dynamicIndexLane]{equal: laneDomain.Equal},
			typedBoundaryFactorOps[dynamicIndexLane]{apply: applyDynamicIndexBoundaryLane, roots: boundaryRootsUnchanged[dynamicIndexLane](), project: projectDynamicIndexBoundaryFactor, rebase: rebaseDynamicIndexBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[dynamicIndexLane], reachability: emitDynamicIndexReachability},
		)
	},
}

func emitDynamicIndexReachability(program *boundaryReachabilityProgramBuilder, lane dynamicIndexLane) {
	if lane.top {
		return
	}
	for factKey, fact := range lane.values {
		if program.pathCone(false, factKey.Table) {
			program.addValue(fact.KeyValue)
			program.addValue(fact.Value)
		}
	}
}
