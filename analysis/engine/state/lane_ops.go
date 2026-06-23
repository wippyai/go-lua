package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

type stateLaneFactory struct {
	id            LaneID
	build         func(*axis.Registry) stateLaneOps
	markReachable func(State) State
}

type stateLaneOps struct {
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
) stateLaneOps {
	return stateLaneOps{
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

func domainFromLaneFactories(reg *axis.Registry, factories []stateLaneFactory) lattice.Lattice[State] {
	lanes := make([]stateLaneOps, 0, len(factories))
	for _, factory := range factories {
		lanes = append(lanes, factory.build(reg))
	}
	return lattice.Lattice[State]{
		Bottom: func() State {
			var out State
			for _, lane := range lanes {
				out = lane.bottom(out)
			}
			return out
		},
		Top: func() State {
			var out State
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
			var out State
			for _, lane := range lanes {
				out = lane.join(out, a, b)
			}
			return out
		},
		Widen: func(prev, next State) State {
			var out State
			for _, lane := range lanes {
				out = lane.widen(out, prev, next)
			}
			return out
		},
	}
}
