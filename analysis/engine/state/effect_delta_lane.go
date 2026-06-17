package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
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
	if key.Target == "" {
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
	if delta.Change == effectdelta.ChangeBottom {
		panic("state: effect delta lane with requires non-bottom delta")
	}
	return effectDeltaLane{l.mapLane.with(key, delta)}
}
