package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

type effectDeltaLane struct {
	values map[effectdelta.Key]effectdelta.Value
	top    bool
}

func effectDeltaLaneFromMap(
	domain lattice.Lattice[map[effectdelta.Key]effectdelta.Value],
	values map[effectdelta.Key]effectdelta.Value,
) effectDeltaLane {
	if domain.Equal(values, domain.Top()) {
		return effectDeltaLane{top: true}
	}
	return effectDeltaLane{values: values}
}

func (l effectDeltaLane) asMap(domain lattice.Lattice[map[effectdelta.Key]effectdelta.Value]) map[effectdelta.Key]effectdelta.Value {
	if l.top {
		return domain.Top()
	}
	return l.values
}

func (l effectDeltaLane) read(key effectdelta.Key) effectdelta.Value {
	if key.Target == "" {
		return effectdelta.Value{}
	}
	if l.top {
		return effectdelta.Top()
	}
	if delta, ok := l.values[key]; ok {
		return delta
	}
	return effectdelta.Value{}
}

func (l effectDeltaLane) hasFinite(key effectdelta.Key) bool {
	if l.top {
		return false
	}
	_, ok := l.values[key]
	return ok
}

func (l effectDeltaLane) without(key effectdelta.Key) (effectDeltaLane, bool) {
	values, changed := effectdelta.DeleteEntry(l.values, key)
	if !changed {
		return l, false
	}
	l.values = values
	return l, true
}

func (l effectDeltaLane) with(key effectdelta.Key, delta effectdelta.Value) effectDeltaLane {
	values := effectdelta.CloneMap(l.values)
	if values == nil {
		values = make(map[effectdelta.Key]effectdelta.Value, 1)
	}
	values[key] = delta
	l.values = values
	return l
}

func (l effectDeltaLane) cloneValues() map[effectdelta.Key]effectdelta.Value {
	if l.top {
		return nil
	}
	return effectdelta.CloneMap(l.values)
}
