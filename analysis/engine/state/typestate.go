package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

// TypestateResourceFromCanonicalKey identifies one resource in state by an
// already-canonical state key and protocol. Callers that have an arbitrary path
// key should use CanonicalTypestateResource or CanonicalTypestateResourceKey so
// proven aliases fold into the same lifecycle resource.
func TypestateResourceFromCanonicalKey(target pathaddr.StateKey, protocol typestate.Protocol) typestate.Resource {
	return typestate.Resource{ID: typestate.ResourceID(target.String()), Protocol: protocol}
}

// CanonicalTypestateResource returns the deterministic resource identity for a
// protocol target. Proven path-equality facts are folded into the same resource
// key so typestate transitions through aliases discharge the original
// obligation.
func (s State) CanonicalTypestateResource(ks *keyspace.KeySpace, target pathaddr.StateKey, protocol typestate.Protocol) typestate.Resource {
	return TypestateResourceFromCanonicalKey(s.CanonicalTypestateResourceKey(ks, target), protocol)
}

// CanonicalTypestateResourceKey returns the stable representative for a state
// key under proven path equality.
func (s State) CanonicalTypestateResourceKey(ks *keyspace.KeySpace, target pathaddr.StateKey) pathaddr.StateKey {
	if target == "" {
		return ""
	}
	canonical := fieldCanonicalTypestatePathKey(ks, target.PathKey())
	for _, equivalent := range s.EquivalentPathKeys(ks, target.PathKey()) {
		equivalent = fieldCanonicalTypestatePathKey(ks, equivalent)
		if equivalent != "" && equivalent < canonical {
			canonical = equivalent
		}
	}
	out, ok := pathaddr.StateKeyFromPathKey(canonical)
	if !ok {
		return ""
	}
	return out
}

func fieldCanonicalTypestatePathKey(ks *keyspace.KeySpace, key pathdom.PathKey) pathdom.PathKey {
	if ks == nil || key == "" {
		return key
	}
	keyStruct, ok := ks.FromStateKey(key)
	if !ok {
		return key
	}
	canonical, ok := ks.FieldCanonical(keyStruct)
	if !ok {
		return key
	}
	formatted := ks.Format(canonical)
	if formatted == "" {
		return key
	}
	return formatted
}

// CanonicalizeTypestateResources folds already-tracked resources through the
// current path-equality evidence. Use this immediately after adding new
// equality evidence so resources acquired before the proof are stored under the
// same canonical identity as later transitions through the alias.
func (s State) CanonicalizeTypestateResources(ks *keyspace.KeySpace) State {
	if ks == nil || !s.laneEnabled(laneTypestatesBit) {
		return s
	}
	next := s.typestates.MapResources(func(resource typestate.Resource) typestate.Resource {
		target, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(resource.ID.String()))
		if !ok {
			return resource
		}
		return TypestateResourceFromCanonicalKey(s.CanonicalTypestateResourceKey(ks, target), resource.Protocol)
	})
	if typestate.Equal(next, s.typestates) {
		return s
	}
	out := s.reachable()
	out.typestates = next
	return out
}

// TypestateSnapshot returns a copy of the current typestate lane.
func (s State) TypestateSnapshot() typestate.Store {
	if !s.laneEnabled(laneTypestatesBit) {
		return typestate.Store{}
	}
	return s.typestates.Clone()
}

// TypestateSlot returns the exact tracked slot for resource, if one is known.
func (s State) TypestateSlot(resource typestate.Resource) (typestate.Slot, bool) {
	if !s.laneEnabled(laneTypestatesBit) {
		return typestate.Slot{}, false
	}
	return s.typestates.Lookup(resource)
}

// OpenTypestateObligations returns locally owned lifecycle obligations that
// are not proven closed or escaped.
func (s State) OpenTypestateObligations() []typestate.OpenObligation {
	if !s.laneEnabled(laneTypestatesBit) {
		return nil
	}
	return s.typestates.OpenObligations()
}

// TypestateInvalidTransitions returns proven transition-precondition failures
// retained by the solved typestate store. The opaque site is assigned by the
// call-boundary applier so post-solve readers can report the operation that
// attempted the invalid transition.
func (s State) TypestateInvalidTransitions() []typestate.InvalidTransition {
	if !s.laneEnabled(laneTypestatesBit) {
		return nil
	}
	return s.typestates.InvalidTransitions()
}

// AcquireTypestate records ownership of a protocol resource.
func (s State) AcquireTypestate(resource typestate.Resource, current typestate.State, obligation typestate.Obligation) State {
	if !s.laneEnabled(laneTypestatesBit) {
		return s
	}
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
	if !s.laneEnabled(laneTypestatesBit) {
		return s
	}
	next := s.typestates.Transition(resource, from, to)
	if typestate.Equal(next, s.typestates) {
		return s
	}
	out := s.reachable()
	out.typestates = next
	return out
}

// TransitionTypestateAt is TransitionTypestate with the source call-site
// identity recorded for any proven invalid transition.
func (s State) TransitionTypestateAt(resource typestate.Resource, from, to typestate.State, site uint32) State {
	if !s.laneEnabled(laneTypestatesBit) {
		return s
	}
	next := s.typestates.TransitionAt(resource, from, to, site)
	if typestate.Equal(next, s.typestates) {
		return s
	}
	out := s.reachable()
	out.typestates = next
	return out
}

// EscapeTypestate records that local lifecycle ownership was transferred away.
func (s State) EscapeTypestate(resource typestate.Resource) State {
	if !s.laneEnabled(laneTypestatesBit) {
		return s
	}
	next := s.typestates.Escape(resource)
	if typestate.Equal(next, s.typestates) {
		return s
	}
	out := s.reachable()
	out.typestates = next
	return out
}
