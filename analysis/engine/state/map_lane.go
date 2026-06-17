package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

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
