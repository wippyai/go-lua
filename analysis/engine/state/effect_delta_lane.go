package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

type effectDeltaLane struct {
	mapLane[effectdelta.Key, effectdelta.Value]
}

func effectDeltaLaneFromMap(
	domain lattice.Lattice[map[effectdelta.Key]effectdelta.Value],
	values map[effectdelta.Key]effectdelta.Value,
) effectDeltaLane {
	return effectDeltaLane{mapLaneFromMap(domain, values)}
}

func (l effectDeltaLane) read(key effectdelta.Key) effectdelta.Value {
	if key.Target.Kind == keyspace.KindInvalid {
		return effectdelta.Value{}
	}
	if l.isTop() {
		return effectdelta.Top()
	}
	if delta, ok := l.get(key); ok {
		return delta
	}
	return effectdelta.Value{}
}

func (l effectDeltaLane) without(key effectdelta.Key) (effectDeltaLane, bool) {
	values, changed := l.mapLane.without(key)
	if !changed {
		return l, false
	}
	return effectDeltaLane{values}, true
}

func (l effectDeltaLane) with(key effectdelta.Key, delta effectdelta.Value) effectDeltaLane {
	requireNonBottomLaneValue(delta.Change == effectdelta.ChangeBottom, "effect delta", "delta")
	return effectDeltaLane{l.mapLane.with(key, delta)}
}

func (l effectDeltaLane) rekey(from, to *keyspace.KeySpace) (effectDeltaLane, bool) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return l, false
	}
	if l.top || len(l.values) == 0 {
		return l, true
	}
	if from == nil || to == nil {
		return l, false
	}
	values := make(map[effectdelta.Key]effectdelta.Value, len(l.values))
	for key, value := range l.values {
		target, ok := to.ImportKey(from, key.Target)
		if !ok {
			return l, false
		}
		key.Target = target
		values[key] = value
	}
	l.values = values
	return l, true
}
