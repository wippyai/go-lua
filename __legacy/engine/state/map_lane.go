package state

import (
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/__legacy/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// sortedSetValues returns the keys of a set in stable order under less.
func sortedSetValues[T comparable](in map[T]struct{}, less func(a, b T) bool) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, 0, len(in))
	for v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

// mustSetLane is the canonical state-side storage adapter for a finite must-set
// of T with an explicit-bottom sentinel, bridging to lift.MustSetLane for the
// lattice operations. Per-element validity (zero-key rejection) lives in the
// owning lane's typed wrappers.
type mustSetLane[T comparable] struct {
	bottom bool
	values map[T]struct{}
}

func mustSetLaneFromLift[T comparable](l lift.MustSetLane[T]) mustSetLane[T] {
	// MustSetLane values are immutable once published to the lattice. Keep the
	// persistent representation so Same and operand-reusing lattice operations
	// remain visible through this state-side adapter.
	return mustSetLane[T]{bottom: l.Bottom(), values: l.Values()}
}

func (l mustSetLane[T]) asMustSet() lift.MustSetLane[T] {
	if l.bottom {
		return lift.MustSetBottom[T]()
	}
	return lift.MustSetValues(l.values)
}

func (l mustSetLane[T]) reachable() mustSetLane[T] {
	l.bottom = false
	return l
}

func (l mustSetLane[T]) contains(v T) bool {
	if l.bottom {
		return false
	}
	_, ok := l.values[v]
	return ok
}

// snapshot returns the lane's bottom/top flags and finite values in stable
// order under less. Each owning lane wraps this into its typed snapshot.
func (l mustSetLane[T]) snapshot(less func(a, b T) bool) (bottom bool, top bool, items []T) {
	if l.bottom {
		return true, false, nil
	}
	items = sortedSetValues(l.values, less)
	return false, len(items) == 0, items
}

func (l mustSetLane[T]) insert(v T) (mustSetLane[T], bool) {
	if !l.bottom {
		if _, ok := l.values[v]; ok {
			return l, false
		}
	}
	values := mapedit.Clone(l.values)
	if values == nil {
		values = make(map[T]struct{}, 1)
	}
	values[v] = struct{}{}
	l = l.reachable()
	l.values = values
	return l, true
}

// wrapDomain lifts a lattice over E to a lattice over a newtype wrapper L, given
// the wrap/unwrap isomorphism between L and E. It removes the per-lane domain
// boilerplate shared by the floor lanes and the must-set lanes.
func wrapDomain[E any, L any](
	elem lattice.Lattice[E],
	wrap func(E) L,
	unwrap func(L) E,
) lattice.Lattice[L] {
	combine := func(a, b L, operation func(E, E) E) L {
		aValue, bValue := unwrap(a), unwrap(b)
		result := operation(aValue, bValue)
		if elem.Same != nil {
			switch {
			case elem.Same(result, aValue):
				return a
			case elem.Same(result, bValue):
				return b
			}
		}
		// A lattice operation may rebuild an equal carrier even when its
		// persistent implementation missed operand reuse. Keep the wrapper's
		// existing operand spelling in that exact semantic no-op case.
		switch {
		case elem.Equal(result, aValue):
			return a
		case elem.Equal(result, bValue):
			return b
		}
		return wrap(result)
	}
	out := lattice.Lattice[L]{
		Bottom:   func() L { return wrap(elem.Bottom()) },
		Top:      func() L { return wrap(elem.Top()) },
		Equal:    func(a, b L) bool { return elem.Equal(unwrap(a), unwrap(b)) },
		LessOrEq: func(a, b L) bool { return elem.LessOrEq(unwrap(a), unwrap(b)) },
		Join:     func(a, b L) L { return combine(a, b, elem.Join) },
		Widen:    func(prev, next L) L { return combine(prev, next, elem.Widen) },
	}
	if elem.Same != nil {
		out.Same = func(a, b L) bool { return elem.Same(unwrap(a), unwrap(b)) }
	}
	if elem.Meet != nil {
		out.Meet = func(a, b L) L { return combine(a, b, elem.Meet) }
	}
	if elem.Narrow != nil {
		out.Narrow = func(prev, next L) L { return combine(prev, next, elem.Narrow) }
	}
	return out
}

// mapLane is the canonical state-side storage adapter for a finite map of facts
// keyed by K, with a top sentinel that avoids materializing the (large) top map
// in the common non-top case. The lattice operates on the underlying map[K]V via
// asMap; per-domain policy (zero-key handling, bottom/top values, write
// admission) lives in the owning State accessor, not here.
type mapLane[K comparable, V any] struct {
	values map[K]V
	top    bool
}

func mapLaneFromMap[K comparable, V any](domain lattice.Lattice[map[K]V], values map[K]V) mapLane[K, V] {
	if domain.Equal(values, domain.Top()) {
		return mapLane[K, V]{top: true}
	}
	return mapLane[K, V]{values: values}
}

func (l mapLane[K, V]) asMap(domain lattice.Lattice[map[K]V]) map[K]V {
	if l.top {
		return domain.Top()
	}
	return l.values
}

func (l mapLane[K, V]) isTop() bool { return l.top }

func (l mapLane[K, V]) get(key K) (V, bool) {
	if l.top {
		var zero V
		return zero, false
	}
	value, ok := l.values[key]
	return value, ok
}

func (l mapLane[K, V]) hasFinite(key K) bool {
	if l.top {
		return false
	}
	_, ok := l.values[key]
	return ok
}

func (l mapLane[K, V]) without(key K) (mapLane[K, V], bool) {
	values, changed := mapedit.Without(l.values, key)
	if !changed {
		return l, false
	}
	l.values = values
	return l, true
}

func (l mapLane[K, V]) with(key K, value V) mapLane[K, V] {
	l.values = mapedit.With(l.values, key, value)
	return l
}

func (l mapLane[K, V]) cloneValues() map[K]V {
	if l.top {
		return nil
	}
	return mapedit.Clone(l.values)
}
