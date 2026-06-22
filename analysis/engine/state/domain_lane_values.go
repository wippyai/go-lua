package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

const LaneValues LaneID = "values"

var valuesDomainLane = stateLaneFactory{
	id: LaneValues,
	build: func(reg *axis.Registry) stateLaneOps {
		domain := lift.Map[key.Value, product.Value](product.Domain(reg))
		return stateLane(domain,
			func(s State) map[key.Value]product.Value {
				return s.values.asMap(domain)
			},
			func(out *State, values map[key.Value]product.Value) {
				out.values = valueLaneFromMap(domain, values)
			},
		)
	},
}
