package state

import effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"

func (s State) ReadEffectDelta(key effectdelta.Key) effectdelta.Value {
	if key.Target == "" {
		return effectdelta.Value{}
	}
	if s.effectDeltasTop {
		return effectdelta.Top()
	}
	if delta, ok := s.effectDeltas[key]; ok {
		return delta
	}
	return effectdelta.Value{}
}

func (s State) WriteEffectDelta(key effectdelta.Key, delta effectdelta.Value) State {
	if key.Target == "" {
		return s
	}
	if s.effectDeltasTop {
		panic("state: cannot finite-write effect delta into top effect-delta lane")
	}
	if delta.Change == effectdelta.ChangeBottom {
		deltas, changed := effectdelta.DeleteEntry(s.effectDeltas, key)
		if !changed {
			return s
		}
		out := s.reachable()
		out.effectDeltas = deltas
		return out
	}
	if existing, ok := s.effectDeltas[key]; ok && existing == delta {
		return s
	}
	deltas := effectdelta.CloneMap(s.effectDeltas)
	if deltas == nil {
		deltas = make(map[effectdelta.Key]effectdelta.Value, 1)
	}
	deltas[key] = delta
	out := s.reachable()
	out.effectDeltas = deltas
	return out
}
