package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func readWithCurrentPointState(
	point cfg.Point,
	read func(cfg.Point) state.State,
	current state.State,
) func(cfg.Point) state.State {
	return func(requested cfg.Point) state.State {
		if requested == point {
			return current
		}
		if read == nil {
			return state.State{}
		}
		return read(requested)
	}
}
