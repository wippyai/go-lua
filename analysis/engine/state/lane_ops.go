package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

type laneKeySpaceMode uint8

const (
	laneKeySpaceInvalid laneKeySpaceMode = iota
	laneKeySpaceFree
	laneKeySpaceOwned
)

// laneSpec is the registration unit for one State product-lattice axis.
// It names the axis, builds its lattice operations, and optionally marks the
// lane reachable when a write leaves lattice bottom.
type laneSpec struct {
	id            LaneID
	bit           laneMask
	build         func(*axis.Registry, DomainOptions) laneOps
	markReachable func(State) State
	fingerprint   func(*fingerprintWriter, State)
	keySpaceMode  laneKeySpaceMode
	rekey         func(State, *keyspace.KeySpace, *keyspace.KeySpace) (State, bool)
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
	bottom   func(*State)
	top      func(*State)
	equal    func(State, State) bool
	same     func(State, State) bool
	lessOrEq func(State, State) bool
	join     func(*State, State, State, bool)
	widen    func(*State, State, State, bool)
	narrow   func(*State, State, State)
}

func stateLane[T any](
	domain lattice.Lattice[T],
	get func(State) T,
	set func(*State, T),
) laneOps {
	return laneOps{
		bottom: func(out *State) {
			set(out, domain.Bottom())
		},
		top: func(out *State) {
			set(out, domain.Top())
		},
		equal: func(a, b State) bool {
			return domain.Equal(get(a), get(b))
		},
		same: func(a, b State) bool {
			return domain.Same != nil && domain.Same(get(a), get(b))
		},
		lessOrEq: func(a, b State) bool {
			return domain.LessOrEq(get(a), get(b))
		},
		join: func(out *State, a, b State, reuseInputs bool) {
			av := get(a)
			bv := get(b)
			if reuseInputs {
				switch {
				case domain.Equal(av, bv):
					set(out, av)
					return
				case domain.LessOrEq(av, bv):
					set(out, bv)
					return
				case domain.LessOrEq(bv, av):
					set(out, av)
					return
				}
			}
			set(out, domain.Join(av, bv))
		},
		widen: func(out *State, prev, next State, reuseInputs bool) {
			pv := get(prev)
			nv := get(next)
			if reuseInputs {
				if domain.Equal(pv, nv) || domain.LessOrEq(nv, pv) {
					set(out, pv)
					return
				}
			}
			set(out, domain.Widen(pv, nv))
		},
		narrow: func(out *State, prev, next State) {
			if domain.Narrow == nil {
				set(out, get(prev))
				return
			}
			set(out, domain.Narrow(get(prev), get(next)))
		},
	}
}

func domainFromLaneSpecs(reg *axis.Registry, specs []laneSpec, universe []laneSpec) lattice.Lattice[State] {
	return domainFromLaneSpecsWithOptions(reg, specs, universe, DomainOptions{})
}

func domainFromLaneSpecsWithOptions(reg *axis.Registry, specs []laneSpec, universe []laneSpec, options DomainOptions) lattice.Lattice[State] {
	lanes := make([]laneOps, 0, len(specs))
	var bits laneMask
	for _, spec := range specs {
		lanes = append(lanes, spec.build(reg, options))
		bits |= spec.bit
	}
	bottomLanes := make([]laneOps, 0, len(universe))
	if sameLaneSpecs(specs, universe) {
		bottomLanes = lanes
	} else {
		for _, spec := range universe {
			bottomLanes = append(bottomLanes, spec.build(reg, options))
		}
	}
	scope := scopedLaneMask(bits)
	bottom := State{}
	for _, lane := range bottomLanes {
		lane.bottom(&bottom)
	}
	bottom.laneMask = scope
	bottom.canonical = true
	return lattice.Lattice[State]{
		Bottom: func() State {
			return bottom
		},
		Top: func() State {
			out := bottom
			for _, lane := range lanes {
				lane.top(&out)
			}
			out.canonical = true
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
			reuseInputs := a.canonical && b.canonical
			if reuseInputs {
				switch {
				case lanesEqual(lanes, a, b) && stateHasLaneMask(a, scope):
					return a
				case lanesLessOrEq(lanes, a, b) && stateHasLaneMask(b, scope):
					return b
				case lanesLessOrEq(lanes, b, a) && stateHasLaneMask(a, scope):
					return a
				}
			}
			out := bottom
			for _, lane := range lanes {
				lane.join(&out, a, b, reuseInputs)
			}
			out.canonical = true
			return out
		},
		Widen: func(prev, next State) State {
			out := bottom
			reuseInputs := prev.canonical && next.canonical
			for _, lane := range lanes {
				lane.widen(&out, prev, next, reuseInputs)
			}
			out.canonical = true
			return out
		},
		Narrow: func(prev, next State) State {
			out := bottom
			for _, lane := range lanes {
				lane.narrow(&out, prev, next)
			}
			out.canonical = true
			return out
		},
	}
}

func stateHasLaneMask(s State, mask laneMask) bool {
	return s.laneMask == mask
}

func lanesEqual(lanes []laneOps, a, b State) bool {
	for _, lane := range lanes {
		if !lane.equal(a, b) {
			return false
		}
	}
	return true
}

func lanesLessOrEq(lanes []laneOps, a, b State) bool {
	for _, lane := range lanes {
		if lane.same(a, b) {
			continue
		}
		if !lane.lessOrEq(a, b) {
			return false
		}
	}
	return true
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
