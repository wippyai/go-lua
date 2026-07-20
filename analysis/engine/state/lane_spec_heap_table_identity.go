package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

const LaneHeapTableIdentity LaneID = "heap-table-identity"

var heapTableIdentityLaneSpec = laneSpec{
	id:                       LaneHeapTableIdentity,
	keySpaceMode:             laneKeySpaceOwned,
	valueDependencies:        independentValueDependencies(),
	identitySupport:          enumeratedIdentitySupport(visitHeapTableIdentityLaneIdentities, func(s State) heapTableIdentityLane { return s.heapTableIdentity }, IdentityImagePointwiseMap),
	numericConsistency:       numericConsistencyIndependent(),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionParticipant(), pathEqualityQuotientIndependent(), genericForBindingFixed(true, false, false), pathReplacementLane[heapTableIdentityLane](false, true, true, applyPathReplacementHeapLane), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, true, true),
	dynamicRead:              dynamicReadIndependent(),
	coordinateFamilies:       []coordinateFamilySpec{heapCoordinateFamilySpec},
	boundary:                 boundaryLaneOps{project: projectHeapBoundary, rebase: rebaseHeapBoundary, postRebase: postRebaseBoundaryNoop, equal: equalHeapBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.heapTableIdentity.rekey(from, to)
		if !ok {
			return s, false
		}
		s.heapTableIdentity = lane
		return s, true
	},
	fingerprint: fingerprintHeapTableIdentity,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := heapTermMapDomain(reg)
		laneDomain := wrapDomain(domain,
			func(objects map[identity.Term]heapidentity.TableObject) heapTableIdentityLane {
				return heapTableIdentityLaneFromMap(domain, objects)
			},
			func(lane heapTableIdentityLane) map[identity.Term]heapidentity.TableObject { return lane.asMap(domain) },
		)
		return stateLaneWithBoundary(laneDomain,
			func(s State) heapTableIdentityLane { return s.heapTableIdentity },
			func(out *State, lane heapTableIdentityLane) { out.heapTableIdentity = lane },
			typedLaneFactorRepresentation[heapTableIdentityLane]{equal: laneDomain.Equal},
			typedBoundaryFactorOps[heapTableIdentityLane]{apply: applyHeapBoundaryLane, roots: boundaryRootsUnchanged[heapTableIdentityLane](), project: projectHeapBoundaryFactor, rebase: rebaseHeapBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[heapTableIdentityLane], reachability: emitHeapTableIdentityReachability},
		)
	},
}

func emitHeapTableIdentityReachability(program *boundaryReachabilityProgramBuilder, lane heapTableIdentityLane) {
	if lane.top {
		return
	}
	for owner, object := range lane.values {
		if !program.identity(owner) {
			continue
		}
		program.addValue(object.Root())
		object.VisitStaticMembers(func(path keyspace.Key, value product.Value) bool {
			program.addHeapSuffix(owner, path)
			program.addValue(value)
			return true
		})
		object.VisitDynamicIndexFacts(func(factKey dynamicindex.Key, fact dynamicindex.Fact) bool {
			program.addHeapSuffix(owner, factKey.Table)
			program.addValue(fact.KeyValue)
			program.addValue(fact.Value)
			return true
		})
	}
}
