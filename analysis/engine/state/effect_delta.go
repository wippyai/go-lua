package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

func (s State) ReadEffectDelta(key effectdelta.Key) effectdelta.Value {
	return s.effectDeltas.read(key)
}

func (s State) WriteEffectDelta(key effectdelta.Key, delta effectdelta.Value) State {
	if key.Target.Kind == keyspace.KindInvalid {
		return s
	}
	if s.effectDeltas.top {
		panic("state: cannot finite-write effect delta into top effect-delta lane")
	}
	if delta.Change == effectdelta.ChangeBottom {
		deltas, changed := s.effectDeltas.without(key)
		if !changed {
			return s
		}
		out := s.reachable()
		out.effectDeltas = deltas
		return out
	}
	if s.effectDeltas.read(key) == delta {
		return s
	}
	out := s.reachable()
	out.effectDeltas = s.effectDeltas.with(key, delta)
	return out
}
