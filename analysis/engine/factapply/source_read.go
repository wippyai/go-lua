package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func readWithSamePointCallSource(
	point cfg.Point,
	source factflow.ValueSource,
	read func(cfg.Point) state.State,
	current state.State,
) func(cfg.Point) state.State {
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != point {
		return read
	}
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
