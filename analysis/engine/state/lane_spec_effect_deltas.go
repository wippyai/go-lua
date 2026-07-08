package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

const LaneEffectDeltas LaneID = "effect-deltas"

var effectDeltasLaneSpec = laneSpec{
	id: LaneEffectDeltas,
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
