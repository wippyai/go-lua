package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/lattice"
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

// joinPathPresence is the least-upper-bound on the 4-element presence lattice.
//
// Polarity (per PRESENCE_DOMAIN_DESIGN.md §3 rev 2 / Codex amendment): Unknown
// is Bottom (the join identity, γ = ∅, "no information yet"), Maybe is Top (the
// absorbing element, γ = full state space). Present and Absent are incomparable
// mid-level elements.
//
// Table:
//
//	Unknown ⊔ x       = x                 (Unknown is the join identity / Bottom)
//	Maybe   ⊔ x       = Maybe              (Maybe is the absorbing element / Top)
//	Present ⊔ Present = Present
//	Absent  ⊔ Absent  = Absent
//	Present ⊔ Absent  = Maybe
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

// meetPathPresence is the greatest-lower-bound on the 4-element presence
// lattice. Dual to joinPathPresence (per PRESENCE_DOMAIN_DESIGN.md §5 rev 2).
//
// Table:
//
//	Unknown ⊓ x       = Unknown            (Unknown is Bottom; meet absorbs)
//	Maybe   ⊓ x       = x                  (Maybe is Top; meet is identity)
//	Present ⊓ Present = Present
//	Absent  ⊓ Absent  = Absent
//	Present ⊓ Absent  = Unknown            (no shared concretization)
func meetPathPresence(a, b pathPresence) pathPresence {
	if a == pathPresenceUnknown || b == pathPresenceUnknown {
		return pathPresenceUnknown
	}
	if a == pathPresenceMaybe {
		return b
	}
	if b == pathPresenceMaybe {
		return a
	}
	if a == b {
		return a
	}
	return pathPresenceUnknown
}

// pathPresenceDomain wires the 4-element presence carrier to the
// lattice.Lattice contract. Per PRESENCE_DOMAIN_DESIGN.md §6 rev 2:
//
//   - Bottom = pathPresenceUnknown (the join identity)
//   - Top    = pathPresenceMaybe   (the join absorbing element)
//   - Widen  = Join: the lattice has finite height 2, so the trivial widening
//     suffices (longest strict chain Unknown → Present|Absent → Maybe is 2
//     steps). No Cousot extrapolation needed.
//
// Per the "no adapter" directive: the contract IS a struct of function fields;
// the fields point at the existing/new package functions directly.
var pathPresenceDomain = lattice.Lattice[pathPresence]{
	Bottom:   func() pathPresence { return pathPresenceUnknown },
	Top:      func() pathPresence { return pathPresenceMaybe },
	Equal:    func(a, b pathPresence) bool { return a == b },
	LessOrEq: func(a, b pathPresence) bool { return joinPathPresence(a, b) == b },
	Join:     joinPathPresence,
	Meet:     meetPathPresence,
	Widen:    joinPathPresence,
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
	_, _, ok := indexedPathSuffix(constraint.PathKey(key))
	return ok
}

func presencePathKey(key string) (constraint.PathKey, bool) {
	if key == "" || !isPresenceKey(key) {
		return "", false
	}
	return constraint.PathKey(key), true
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
	pkey, ok := presencePathKey(key)
	if s == nil || !ok {
		return pathPresenceUnknown
	}
	if state := s.mutablePresence[p]; state != nil {
		if presence, ok := state[pkey]; ok {
			return presence
		}
	}
	if presence, ok := s.presence[pkey]; ok {
		return presence
	}
	return pathPresenceFromType(s.valueAtPoint(p, key))
}

func (s *Solution) setValuePresence(key string, t typ.Type) {
	pkey, ok := presencePathKey(key)
	if s == nil || !ok {
		return
	}
	presence := pathPresenceFromType(t)
	if presence == pathPresenceUnknown {
		return
	}
	if s.presence == nil {
		s.presence = make(map[constraint.PathKey]pathPresence, 1)
	}
	s.presence[pkey] = presence
}

func (s *Solution) setMutablePresence(p cfg.Point, key string, t typ.Type) {
	pkey, ok := presencePathKey(key)
	if s == nil || !ok {
		return
	}
	presence := pathPresenceFromType(t)
	if presence == pathPresenceUnknown {
		return
	}
	if s.mutablePresence == nil {
		s.mutablePresence = make(map[cfg.Point]map[constraint.PathKey]pathPresence)
	}
	state := s.mutablePresence[p]
	if state == nil {
		state = make(map[constraint.PathKey]pathPresence, 1)
		s.mutablePresence[p] = state
	}
	state[pkey] = presence
}

func (s *Solution) rebuildMutablePresenceForPoint(p cfg.Point) {
	if s == nil {
		return
	}
	state := s.mutableOut[p]
	if len(state) == 0 {
		if s.mutablePresence != nil {
			delete(s.mutablePresence, p)
		}
		return
	}
	if s.mutablePresence == nil {
		s.mutablePresence = make(map[cfg.Point]map[constraint.PathKey]pathPresence)
	}
	presence := make(map[constraint.PathKey]pathPresence, len(state))
	for key, av := range state {
		pkey, ok := presencePathKey(string(key))
		if !ok {
			continue
		}
		if p := pathPresenceFromType(projectFlowValue(av)); p != pathPresenceUnknown {
			presence[pkey] = p
		}
	}
	if len(presence) == 0 {
		delete(s.mutablePresence, p)
		return
	}
	s.mutablePresence[p] = presence
}
