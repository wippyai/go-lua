package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

// mustSetLane is the canonical state-side storage adapter for a finite must-set
// of T with an explicit-bottom sentinel, bridging to lift.MustSetLane for the
// lattice operations. Per-element validity (zero-key rejection) lives in the
// owning lane's typed wrappers.
type mustSetLane[T comparable] struct {
	bottom bool
	values map[T]struct{}
}

func mustSetLaneFromLift[T comparable](l lift.MustSetLane[T]) mustSetLane[T] {
	return mustSetLane[T]{bottom: l.Bottom(), values: mapedit.Clone(l.Values())}
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
	return lattice.Lattice[L]{
		Bottom:   func() L { return wrap(elem.Bottom()) },
		Top:      func() L { return wrap(elem.Top()) },
		Equal:    func(a, b L) bool { return elem.Equal(unwrap(a), unwrap(b)) },
		LessOrEq: func(a, b L) bool { return elem.LessOrEq(unwrap(a), unwrap(b)) },
		Join:     func(a, b L) L { return wrap(elem.Join(unwrap(a), unwrap(b))) },
		Widen:    func(prev, next L) L { return wrap(elem.Widen(unwrap(prev), unwrap(next))) },
	}
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
