package state

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ValueFactor is the exact lifted finite-map lattice used by scalar slots.
// Top is explicit because the key universe is not enumerated; otherwise a
// missing slot denotes product bottom. K is address syntax only—the value
// lattice is identical for concrete State slots and formal relation slots.
type ValueFactor[K comparable] struct {
	Top    bool
	Values map[K]product.Value
}

// ValueLaneFactor preserves the concrete State API while sharing the one
// generic scalar-map implementation with the formal relation carrier.
type ValueLaneFactor = ValueFactor[key.Value]

// ValueFactorLattice returns the exact product-value map lattice for any
// collision-free comparable slot vocabulary.
func ValueFactorLattice[K comparable](reg *axis.Registry) lattice.Lattice[ValueFactor[K]] {
	values := lift.Map[K, product.Value](product.Domain(reg))
	domain := lattice.Lattice[ValueFactor[K]]{
		Bottom: func() ValueFactor[K] { return ValueFactor[K]{} },
		Top:    func() ValueFactor[K] { return ValueFactor[K]{Top: true} },
		Equal: func(a, b ValueFactor[K]) bool {
			if a.Top || b.Top {
				return a.Top && b.Top
			}
			return values.Equal(a.Values, b.Values)
		},
		Same: func(a, b ValueFactor[K]) bool {
			if a.Top || b.Top {
				return a.Top && b.Top
			}
			return values.Same(a.Values, b.Values)
		},
		LessOrEq: func(a, b ValueFactor[K]) bool {
			switch {
			case b.Top:
				return true
			case a.Top:
				return false
			default:
				return values.LessOrEq(a.Values, b.Values)
			}
		},
		Join: func(a, b ValueFactor[K]) ValueFactor[K] {
			if a.Top || b.Top {
				return ValueFactor[K]{Top: true}
			}
			return ValueFactor[K]{Values: values.Join(a.Values, b.Values)}
		},
		Widen: func(previous, next ValueFactor[K]) ValueFactor[K] {
			if previous.Top || next.Top {
				return ValueFactor[K]{Top: true}
			}
			return ValueFactor[K]{Values: values.Widen(previous.Values, next.Values)}
		},
	}
	if values.Meet != nil {
		domain.Meet = func(a, b ValueFactor[K]) ValueFactor[K] {
			switch {
			case a.Top:
				return b
			case b.Top:
				return a
			default:
				return ValueFactor[K]{Values: values.Meet(a.Values, b.Values)}
			}
		}
	}
	if values.Narrow != nil {
		domain.Narrow = func(previous, next ValueFactor[K]) ValueFactor[K] {
			switch {
			case previous.Top:
				return next
			case next.Top:
				return previous
			default:
				return ValueFactor[K]{Values: values.Narrow(previous.Values, next.Values)}
			}
		}
	} else {
		// Whole-State narrowing keeps the previous component when a registered
		// lane has no narrowing operator. Formal Values uses this generic lattice
		// directly, so retain the same total law instead of exposing nil.
		domain.Narrow = func(previous, _ ValueFactor[K]) ValueFactor[K] {
			return previous
		}
	}
	return domain
}

// DecomposeValueLane transposes the Values component out of State while
// preserving every other enabled lane. The returned residual belongs to the
// same State domain and has Values at lattice bottom.
func DecomposeValueLane(domain lattice.Lattice[State], value State) (State, ValueLaneFactor) {
	if !value.canonical {
		value = NormalizeForDomain(domain, value)
	}
	snapshot := value.ValuesSnapshot()
	value.values = valueLane{}
	return value, ValueLaneFactor{Top: snapshot.Top, Values: snapshot.Values}
}

// RecomposeValueLane is the inverse of DecomposeValueLane. It is used at an
// operation boundary after guarded slot roots have been aligned; ordinary
// solver storage keeps the components factored and never materializes their
// Cartesian product.
func RecomposeValueLane(reg *axis.Registry, domain lattice.Lattice[State], residual State, factor ValueLaneFactor) State {
	out := residual
	if !out.canonical {
		out = NormalizeForDomain(domain, out)
	}
	out.values = valueLaneFromFactor(reg, factor)
	// Values were assembled directly in the canonical lifted-map spelling:
	// Top is unique, finite maps omit Bottom, and symbol/return namespaces are
	// disjoint. The residual was normalized above, so joining the complete
	// 17-lane product from Bottom again cannot change semantics and only repeats
	// every unrelated lane operation.
	out.canonical = true
	return out
}

// ComposeFactorTuple closes the transposed complete product in one registered
// ProductDomain.Compose call. Values is supplied through its exact slot
// factor while Factors must be the complete NonValuesLaneInventory in catalog
// order. No caller needs to know where the slot-factored carrier occurs among
// registered lanes.
func (d ProductDomain) ComposeFactorTuple(values ValueLaneFactor, factors []LaneFactor) (State, error) {
	if !d.Valid() || len(factors) != d.NonValuesLaneCount() {
		return State{}, ErrIncompleteLaneFactors
	}
	all := make([]LaneFactor, len(d.factorLanes))
	nonValues := 0
	valueState := RecomposeValueLane(d.reg, d.lattice, d.lattice.Bottom(), values)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		if runtime.lane.slotFactored {
			all[index] = LaneFactor{lane: runtime.lane, payload: runtime.ops.extract(valueState)}
			continue
		}
		if nonValues >= len(factors) || factors[nonValues].Lane() != runtime.lane {
			return State{}, ErrIncompleteLaneFactors
		}
		all[index] = factors[nonValues]
		nonValues++
	}
	if nonValues != len(factors) {
		return State{}, ErrIncompleteLaneFactors
	}
	return d.Compose(all)
}

// JoinFactorTuples returns the componentwise least upper bound of two complete
// factored products without constructing either State. Residual factors must
// follow NonValuesLaneInventory exactly. ComposeFactorTuple is the sole fence
// needed after any number of these joins.
func (d ProductDomain) JoinFactorTuples(
	leftValues ValueLaneFactor,
	leftFactors []LaneFactor,
	rightValues ValueLaneFactor,
	rightFactors []LaneFactor,
) (ValueLaneFactor, []LaneFactor, error) {
	if !d.Valid() || len(leftFactors) != d.NonValuesLaneCount() || len(rightFactors) != d.NonValuesLaneCount() {
		return ValueLaneFactor{}, nil, ErrIncompleteLaneFactors
	}
	joined := make([]LaneFactor, len(leftFactors))
	for index := range leftFactors {
		lane, ok := d.NonValuesLaneAt(index)
		if !ok || leftFactors[index].Lane() != lane || rightFactors[index].Lane() != lane {
			return ValueLaneFactor{}, nil, ErrIncompleteLaneFactors
		}
		runtime, err := d.validateFactorPair(leftFactors[index], rightFactors[index])
		if err != nil {
			return ValueLaneFactor{}, nil, err
		}
		// Publication folds complete tuples whose terminal rows commonly share
		// persistent lane carriers. Representation identity is a proof of exact
		// equality, so retaining that carrier is the same componentwise join
		// without repeating the lane's (often map- or product-heavy) lattice
		// comparison and allocation work.
		if runtime.ops.same(leftFactors[index].payload, rightFactors[index].payload) {
			joined[index] = leftFactors[index]
			continue
		}
		joined[index] = LaneFactor{lane: runtime.lane, payload: runtime.ops.join(leftFactors[index].payload, rightFactors[index].payload)}
	}
	valueDomain := ValueFactorLattice[key.Value](d.reg)
	values := leftValues
	if valueDomain.Same == nil || !valueDomain.Same(leftValues, rightValues) {
		values = valueDomain.Join(leftValues, rightValues)
	}
	return values, joined, nil
}

func valueLaneFromFactor(reg *axis.Registry, factor ValueLaneFactor) valueLane {
	values := valueLane{top: factor.Top}
	if factor.Top {
		return values
	}
	bottom := product.Bottom(reg)
	for slot, value := range factor.Values {
		if slot == 0 || product.Equal(reg, value, bottom) {
			continue
		}
		if valueSlotIsReturn(slot) {
			if values.returns == nil {
				values.returns = make(map[key.Value]product.Value)
			}
			values.returns[slot] = value
			continue
		}
		if values.symbols == nil {
			values.symbols = make(map[key.Value]product.Value)
		}
		values.symbols[slot] = value
	}
	return values
}

func valueLaneToFactor(values valueLane) ValueLaneFactor {
	return ValueLaneFactor{Top: values.top, Values: values.cloneValues()}
}
