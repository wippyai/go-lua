package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// laneSpec is the registration unit for one State product-lattice axis.
// It names the axis, builds its lattice operations, and optionally marks the
// lane reachable when a write leaves lattice bottom.
type laneSpec struct {
	id            LaneID
	bit           laneMask
	build         func(*axis.Registry) laneOps
	markReachable func(State) State
}

type laneMask uint64

const laneMaskScoped laneMask = 1 << 63

func scopedLaneMask(bits laneMask) laneMask {
	return laneMaskScoped | bits
}

func (m laneMask) allows(bit laneMask) bool {
	if m&laneMaskScoped == 0 {
		return true
	}
	return m&bit != 0
}

type laneOps struct {
	bottom   func(State) State
	top      func(State) State
	equal    func(State, State) bool
	lessOrEq func(State, State) bool
	join     func(State, State, State) State
	widen    func(State, State, State) State
}

func stateLane[T any](
	domain lattice.Lattice[T],
	get func(State) T,
	set func(*State, T),
) laneOps {
	return laneOps{
		bottom: func(out State) State {
			set(&out, domain.Bottom())
			return out
		},
		top: func(out State) State {
			set(&out, domain.Top())
			return out
		},
		equal: func(a, b State) bool {
			return domain.Equal(get(a), get(b))
		},
		lessOrEq: func(a, b State) bool {
			return domain.LessOrEq(get(a), get(b))
		},
		join: func(out, a, b State) State {
			set(&out, domain.Join(get(a), get(b)))
			return out
		},
		widen: func(out, prev, next State) State {
			set(&out, domain.Widen(get(prev), get(next)))
			return out
		},
	}
}

func domainFromLaneSpecs(reg *axis.Registry, specs []laneSpec, universe []laneSpec) lattice.Lattice[State] {
	lanes := make([]laneOps, 0, len(specs))
	var bits laneMask
	for _, spec := range specs {
		lanes = append(lanes, spec.build(reg))
		bits |= spec.bit
	}
	bottomLanes := make([]laneOps, 0, len(universe))
	if sameLaneSpecs(specs, universe) {
		bottomLanes = lanes
	} else {
		for _, spec := range universe {
			bottomLanes = append(bottomLanes, spec.build(reg))
		}
	}
	scope := scopedLaneMask(bits)
	bottom := State{}
	for _, lane := range bottomLanes {
		bottom = lane.bottom(bottom)
	}
	bottom.laneMask = scope
	return lattice.Lattice[State]{
		Bottom: func() State {
			return bottom
		},
		Top: func() State {
			out := bottom
			for _, lane := range lanes {
				out = lane.top(out)
			}
			return out
		},
		Equal: func(a, b State) bool {
			for _, lane := range lanes {
				if !lane.equal(a, b) {
					return false
				}
			}
			return true
		},
		LessOrEq: func(a, b State) bool {
			for _, lane := range lanes {
				if !lane.lessOrEq(a, b) {
					return false
				}
			}
			return true
		},
		Join: func(a, b State) State {
			out := bottom
			for _, lane := range lanes {
				out = lane.join(out, a, b)
			}
			return out
		},
		Widen: func(prev, next State) State {
			out := bottom
			for _, lane := range lanes {
				out = lane.widen(out, prev, next)
			}
			return out
		},
	}
}

func sameLaneSpecs(a, b []laneSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].id != b[i].id || a[i].bit != b[i].bit {
			return false
		}
	}
	return true
}
