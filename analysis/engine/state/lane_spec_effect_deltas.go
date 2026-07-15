package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

const LaneEffectDeltas LaneID = "effect-deltas"

var effectDeltasLaneSpec = laneSpec{
	id:           LaneEffectDeltas,
	keySpaceMode: laneKeySpaceOwned,
	boundary:     boundaryLaneOps{expand: expandEffectDeltasBoundary, project: projectEffectDeltasBoundary, rebase: rebaseEffectDeltasBoundary, apply: applyEffectDeltasBoundary, equal: equalEffectDeltasBoundary},
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
		return stateLane(domain,
			func(s State) map[effectdelta.Key]effectdelta.Value {
				return s.effectDeltas.asMap(domain)
			},
			func(out *State, deltas map[effectdelta.Key]effectdelta.Value) {
				out.effectDeltas = effectDeltaLaneFromMap(domain, deltas)
			},
		)
	},
}

func expandEffectDeltasBoundary(expansion *boundaryClosureExpansion, source State) {
	if source.effectDeltas.top {
		return
	}
	for effectKey, effect := range source.effectDeltas.values {
		if expansion.connect(effectKey.Target) {
			expansion.addValue(effect.Before)
			expansion.addValue(effect.After)
		}
	}
}
