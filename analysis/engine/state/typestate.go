package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

// TypestateResource identifies one resource in state by canonical path key and
// protocol. Callers are responsible for deriving ID from analysis facts rather
// than source spelling.
func TypestateResource(target pathdom.PathKey, protocol typestate.Protocol) typestate.Resource {
	return typestate.Resource{ID: string(target), Protocol: protocol}
}

// CanonicalTypestateResource returns the deterministic resource identity for a
// protocol target. Proven path-equality facts are folded into the same resource
// key so typestate transitions through aliases discharge the original
// obligation.
func (s State) CanonicalTypestateResource(target pathdom.PathKey, protocol typestate.Protocol) typestate.Resource {
	return TypestateResource(s.CanonicalTypestateResourceKey(target), protocol)
}

// CanonicalTypestateResourceKey returns the stable representative for a path key
// under proven path equality.
func (s State) CanonicalTypestateResourceKey(target pathdom.PathKey) pathdom.PathKey {
	if target == "" {
		return ""
	}
	canonical := target
	for _, equivalent := range s.EquivalentPathKeys(target) {
		if equivalent != "" && equivalent < canonical {
			canonical = equivalent
		}
	}
	return canonical
}

// TypestateSnapshot returns a copy of the current typestate lane.
func (s State) TypestateSnapshot() typestate.Store {
	return s.typestates.Clone()
}

// OpenTypestateObligations returns locally owned lifecycle obligations that
// are not proven closed or escaped.
func (s State) OpenTypestateObligations() []typestate.OpenObligation {
	return s.typestates.OpenObligations()
}

// AcquireTypestate records ownership of a protocol resource.
func (s State) AcquireTypestate(resource typestate.Resource, current typestate.State, obligation typestate.Obligation) State {
	next := s.typestates.Acquire(resource, current, obligation)
	if typestate.Equal(next, s.typestates) {
		return s
	}
	out := s.reachable()
	out.typestates = next
	return out
}

// TransitionTypestate records a protocol state transition.
func (s State) TransitionTypestate(resource typestate.Resource, from, to typestate.State) State {
	next := s.typestates.Transition(resource, from, to)
	if typestate.Equal(next, s.typestates) {
		return s
	}
	out := s.reachable()
	out.typestates = next
	return out
}

// EscapeTypestate records that local lifecycle ownership was transferred away.
func (s State) EscapeTypestate(resource typestate.Resource) State {
	next := s.typestates.Escape(resource)
	if typestate.Equal(next, s.typestates) {
		return s
	}
	out := s.reachable()
	out.typestates = next
	return out
}
