package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

const LaneEffectDeltas LaneID = "effect-deltas"

var effectDeltasLaneSpec = laneSpec{
	id:                 LaneEffectDeltas,
	keySpaceMode:       laneKeySpaceOwned,
	formalRekey:        formalRekeyStructural(),
	valueDependencies:  independentValueDependencies(),
	identitySupport:    enumeratedIdentitySupport(visitEffectDeltasLaneIdentities, func(s State) effectDeltaLane { return s.effectDeltas }, IdentityImageEmbeddedValue),
	numericConsistency: numericConsistencyIndependent(),
	semanticLaws: []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorLane(effectFactorDelta,
		func(s State) effectDeltaLane { return s.effectDeltas },
		func(out *State, lane effectDeltaLane) { out.effectDeltas = lane },
		applyEffectDeltaFactor,
	), callBoundaryIndependent()},
	boundaryClosureCompanion: uniqueBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, false, false),
	dynamicRead:              dynamicReadIndependent(),
	boundary:                 boundaryLaneOps{project: projectEffectDeltasBoundary, rebase: rebaseEffectDeltasBoundary, postRebase: postRebaseBoundaryNoop, equal: equalEffectDeltasBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.effectDeltas.rekey(from, to)
		if !ok {
			return s, false
		}
		s.effectDeltas = lane
		return s, true
	},
	fingerprint: fingerprintEffectDeltas,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := effectdelta.MapDomain(reg)
		laneDomain := wrapDomain(domain,
			func(deltas map[effectdelta.Key]effectdelta.Value) effectDeltaLane {
				return effectDeltaLaneFromMap(domain, deltas)
			},
			func(lane effectDeltaLane) map[effectdelta.Key]effectdelta.Value { return lane.asMap(domain) },
		)
		return stateLaneWithBoundary(laneDomain,
			func(s State) effectDeltaLane { return s.effectDeltas },
			func(out *State, lane effectDeltaLane) { out.effectDeltas = lane },
			typedLaneFactorRepresentation[effectDeltaLane]{equal: laneDomain.Equal},
			typedBoundaryFactorOps[effectDeltaLane]{
				apply: applyEffectDeltasBoundaryLane, roots: boundaryRootsUnchanged[effectDeltaLane](),
				project: projectEffectDeltasBoundaryFactor, rebase: rebaseEffectDeltasBoundaryFactor,
				postRebase: boundaryPostRebaseUnchanged[effectDeltaLane], reachability: emitEffectDeltasReachability,
				extendClosure: func(keys *keyspace.KeySpace, source effectDeltaLane, roots boundaryPathMap, base BoundaryClosure) (BoundaryClosure, bool) {
					return withBoundaryEffectFactorCompanions(keys, source, roots, base), true
				},
			},
		)
	},
}

func emitEffectDeltasReachability(program *boundaryReachabilityProgramBuilder, lane effectDeltaLane) {
	if lane.top {
		return
	}
	for effectKey, effect := range lane.values {
		if program.pathCone(true, effectKey.Target) {
			program.addValue(effect.Before)
			program.addValue(effect.After)
		}
	}
}
