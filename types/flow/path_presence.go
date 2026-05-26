package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// pathPresence is the table-key presence component of the flow abstract state.
// Value types answer "what is stored"; presence answers whether the key exists.
type pathPresence uint8

const (
	pathPresenceUnknown pathPresence = iota
	pathPresencePresent
	pathPresenceAbsent
	pathPresenceMaybe
)

func pathPresenceFromType(t typ.Type) pathPresence {
	if t == nil || typ.IsNever(t) {
		return pathPresenceUnknown
	}
	if t.Kind() == kind.Nil {
		return pathPresenceAbsent
	}
	_, nilable := typ.SplitNilableFieldType(t)
	if nilable {
		return pathPresenceMaybe
	}
	return pathPresencePresent
}

func joinPathPresence(a, b pathPresence) pathPresence {
	if a == pathPresenceUnknown {
		return b
	}
	if b == pathPresenceUnknown {
		return a
	}
	if a == b {
		return a
	}
	return pathPresenceMaybe
}

func projectPathPresence(t typ.Type, presence pathPresence) typ.Type {
	switch presence {
	case pathPresencePresent:
		if inner, nilable := typ.SplitNilableFieldType(t); nilable {
			return inner
		}
		return t
	case pathPresenceAbsent:
		return typ.Nil
	case pathPresenceMaybe:
		if inner, nilable := typ.SplitNilableFieldType(t); nilable {
			return typ.NewOptional(inner)
		}
		if t == nil || typ.IsNever(t) {
			return typ.NewOptional(typ.Unknown)
		}
		return typ.NewOptional(t)
	default:
		return t
	}
}

func isPresenceKey(key string) bool {
	_, _, ok := indexedPathSuffix(key)
	return ok
}

func (s *Solution) projectedValueAtPoint(p cfg.Point, key string) typ.Type {
	return s.projectPresenceAtPoint(p, key, s.valueAtPoint(p, key))
}

func (s *Solution) projectPresenceAtPoint(p cfg.Point, key string, t typ.Type) typ.Type {
	if s == nil || key == "" || !isPresenceKey(key) {
		return t
	}
	return projectPathPresence(t, s.presenceAtPoint(p, key))
}

func (s *Solution) presenceAtPoint(p cfg.Point, key string) pathPresence {
	if s == nil || key == "" || !isPresenceKey(key) {
		return pathPresenceUnknown
	}
	if state := s.mutablePresence[p]; state != nil {
		if presence, ok := state[key]; ok {
			return presence
		}
	}
	if presence, ok := s.presence[key]; ok {
		return presence
	}
	return pathPresenceFromType(s.valueAtPoint(p, key))
}

func (s *Solution) setValuePresence(key string, t typ.Type) {
	if s == nil || key == "" || !isPresenceKey(key) {
		return
	}
	presence := pathPresenceFromType(t)
	if presence == pathPresenceUnknown {
		return
	}
	if s.presence == nil {
		s.presence = make(map[string]pathPresence, 1)
	}
	s.presence[key] = presence
}

func (s *Solution) setMutablePresence(p cfg.Point, key string, t typ.Type) {
	if s == nil || key == "" || !isPresenceKey(key) {
		return
	}
	presence := pathPresenceFromType(t)
	if presence == pathPresenceUnknown {
		return
	}
	if s.mutablePresence == nil {
		s.mutablePresence = make(map[cfg.Point]map[string]pathPresence)
	}
	state := s.mutablePresence[p]
	if state == nil {
		state = make(map[string]pathPresence, 1)
		s.mutablePresence[p] = state
	}
	state[key] = presence
}

func (s *Solution) rebuildMutablePresenceForPoint(p cfg.Point) {
	if s == nil {
		return
	}
	state := s.mutableValues[p]
	if len(state) == 0 {
		if s.mutablePresence != nil {
			delete(s.mutablePresence, p)
		}
		return
	}
	if s.mutablePresence == nil {
		s.mutablePresence = make(map[cfg.Point]map[string]pathPresence)
	}
	presence := make(map[string]pathPresence, len(state))
	for key, av := range state {
		if !isPresenceKey(key) {
			continue
		}
		if p := pathPresenceFromType(projectFlowValue(av)); p != pathPresenceUnknown {
			presence[key] = p
		}
	}
	if len(presence) == 0 {
		delete(s.mutablePresence, p)
		return
	}
	s.mutablePresence[p] = presence
}
