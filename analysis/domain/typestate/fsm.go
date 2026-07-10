package typestate

import (
	"fmt"
	"sort"
)

// TransitionDecl declares one allowed edge in a protocol state machine.
type TransitionDecl struct {
	From State
	To   State
}

// Definition declares the finite state machine for one typestate protocol.
// Runtime analysis still tracks abstract resources by protocol and state; this
// declaration is the module-boundary contract that makes those names explicit.
type Definition struct {
	Protocol    Protocol
	States      []State
	FinalStates []State
	Transitions []TransitionDecl
}

// Clone returns an independent definition copy.
func (d Definition) Clone() Definition {
	return Definition{
		Protocol:    d.Protocol,
		States:      append([]State(nil), d.States...),
		FinalStates: append([]State(nil), d.FinalStates...),
		Transitions: append([]TransitionDecl(nil), d.Transitions...),
	}
}

// Normalized returns d with deterministic state/final/transition order and
// duplicate entries removed. Validation is intentionally separate so callers can
// produce precise errors for malformed declarations.
func (d Definition) Normalized() Definition {
	out := d.Clone()
	out.States = uniqueStates(out.States)
	out.FinalStates = uniqueStates(out.FinalStates)
	out.Transitions = uniqueTransitions(out.Transitions)
	return out
}

// Validate rejects malformed FSM declarations.
func (d Definition) Validate() error {
	if d.Protocol == "" {
		return fmt.Errorf("typestate protocol missing name")
	}
	if len(d.States) == 0 {
		return fmt.Errorf("typestate protocol %q has no states", d.Protocol)
	}
	states := make(map[State]struct{}, len(d.States))
	for _, state := range d.States {
		if state == "" {
			return fmt.Errorf("typestate protocol %q has empty state", d.Protocol)
		}
		if _, exists := states[state]; exists {
			return fmt.Errorf("typestate protocol %q duplicates state %q", d.Protocol, state)
		}
		states[state] = struct{}{}
	}
	finals := make(map[State]struct{}, len(d.FinalStates))
	for _, state := range d.FinalStates {
		if state == "" {
			return fmt.Errorf("typestate protocol %q has empty final state", d.Protocol)
		}
		if _, exists := states[state]; !exists {
			return fmt.Errorf("typestate protocol %q final state %q is not declared", d.Protocol, state)
		}
		if _, exists := finals[state]; exists {
			return fmt.Errorf("typestate protocol %q duplicates final state %q", d.Protocol, state)
		}
		finals[state] = struct{}{}
	}
	transitions := make(map[TransitionDecl]struct{}, len(d.Transitions))
	for _, transition := range d.Transitions {
		if transition.From == "" || transition.To == "" {
			return fmt.Errorf("typestate protocol %q has transition with empty endpoint", d.Protocol)
		}
		if _, exists := states[transition.From]; !exists {
			return fmt.Errorf("typestate protocol %q transition source %q is not declared", d.Protocol, transition.From)
		}
		if _, exists := states[transition.To]; !exists {
			return fmt.Errorf("typestate protocol %q transition target %q is not declared", d.Protocol, transition.To)
		}
		if _, exists := transitions[transition]; exists {
			return fmt.Errorf("typestate protocol %q duplicates transition %q -> %q", d.Protocol, transition.From, transition.To)
		}
		transitions[transition] = struct{}{}
	}
	return nil
}

// HasState reports whether state is declared in this FSM.
func (d Definition) HasState(state State) bool {
	if state == "" {
		return false
	}
	for _, candidate := range d.States {
		if candidate == state {
			return true
		}
	}
	return false
}

// IsFinal reports whether state is declared as final.
func (d Definition) IsFinal(state State) bool {
	if state == "" {
		return false
	}
	for _, candidate := range d.FinalStates {
		if candidate == state {
			return true
		}
	}
	return false
}

// AllowsTransition reports whether the concrete from -> to edge is declared.
func (d Definition) AllowsTransition(from, to State) bool {
	if from == "" || to == "" {
		return false
	}
	for _, transition := range d.Transitions {
		if transition.From == from && transition.To == to {
			return true
		}
	}
	return false
}

func uniqueStates(in []State) []State {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[State]struct{}, len(in))
	out := make([]State, 0, len(in))
	for _, state := range in {
		if _, ok := seen[state]; ok {
			continue
		}
		seen[state] = struct{}{}
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func uniqueTransitions(in []TransitionDecl) []TransitionDecl {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[TransitionDecl]struct{}, len(in))
	out := make([]TransitionDecl, 0, len(in))
	for _, transition := range in {
		if _, ok := seen[transition]; ok {
			continue
		}
		seen[transition] = struct{}{}
		out = append(out, transition)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}
