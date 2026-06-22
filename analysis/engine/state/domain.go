package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

// LaneID names one State product-lattice axis.
type LaneID string

var domainCache registrycache.Cache[lattice.Lattice[State]]

// Domain builds the default State lattice with every state axis enabled.
func Domain(reg *axis.Registry) lattice.Lattice[State] {
	return domainCache.Get(reg, func() lattice.Lattice[State] {
		return domainFromLaneFactories(reg, defaultDomainLaneFactories)
	})
}

// DomainWithLaneSet builds a State lattice from the exact ordered set of
// enabled lanes. Disabled lanes are ignored by Equal/LessOrEq and dropped by
// Join/Widen.
func DomainWithLaneSet(reg *axis.Registry, lanes LaneSet) lattice.Lattice[State] {
	return domainFromLaneFactories(reg, selectDomainLaneFactories(lanes))
}

// DomainWithLanes is the compatibility form of DomainWithLaneSet.
func DomainWithLanes(reg *axis.Registry, lanes []LaneID) lattice.Lattice[State] {
	return DomainWithLaneSet(reg, LaneSet(lanes))
}

type stateLaneFactory struct {
	id    LaneID
	build func(*axis.Registry) stateLaneOps
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

func selectDomainLaneFactories(lanes LaneSet) []stateLaneFactory {
	byID := make(map[LaneID]stateLaneFactory, len(defaultDomainLaneFactories))
	for _, factory := range defaultDomainLaneFactories {
		byID[factory.id] = factory
	}
	seen := make(map[LaneID]struct{}, len(lanes))
	out := make([]stateLaneFactory, 0, len(lanes))
	for _, id := range lanes {
		factory, ok := byID[id]
		if !ok {
			panic("state: unknown domain lane " + string(id))
		}
		if _, ok := seen[id]; ok {
			panic("state: duplicate domain lane " + string(id))
		}
		seen[id] = struct{}{}
		out = append(out, factory)
	}
	return out
}
