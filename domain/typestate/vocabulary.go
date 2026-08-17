package typestate

import (
	"strconv"
	"strings"
)

// Protocol identifies one declared state-machine namespace.
type Protocol string

// State identifies one declared protocol state.
type State string

// FinalStates is a canonical, comparable set of states that can discharge an
// obligation. It remains manifest vocabulary; Link owns the analyzer's
// structural protocol-state authority.
type FinalStates string

// NewFinalStates returns a deterministic set of non-empty final states.
func NewFinalStates(states ...State) FinalStates {
	if len(states) == 0 {
		return ""
	}
	unique := uniqueStates(states)
	if len(unique) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, state := range unique {
		raw := state.String()
		builder.WriteString(strconv.Itoa(len(raw)))
		builder.WriteByte(':')
		builder.WriteString(raw)
	}
	return FinalStates(builder.String())
}

// States decodes the final states in deterministic order.
func (states FinalStates) States() []State {
	if states == "" {
		return nil
	}
	raw := string(states)
	out := make([]State, 0, 2)
	for len(raw) > 0 {
		colon := strings.IndexByte(raw, ':')
		if colon <= 0 {
			return nil
		}
		length, err := strconv.Atoi(raw[:colon])
		if err != nil || length <= 0 || colon+1+length > len(raw) {
			return nil
		}
		out = append(out, State(raw[colon+1:colon+1+length]))
		raw = raw[colon+1+length:]
	}
	return out
}

// Contains reports whether state is one of the satisfying states.
func (states FinalStates) Contains(state State) bool {
	if state == "" || states == "" {
		return false
	}
	for _, candidate := range states.States() {
		if candidate == state {
			return true
		}
	}
	return false
}

// Obligation describes the manifest-declared states that discharge a
// lifecycle obligation. Final is retained for the canonical manifest shape;
// Finals is authoritative when present.
type Obligation struct {
	Final  State
	Finals FinalStates
}

// SatisfiedBy reports whether state discharges this obligation.
func (obligation Obligation) SatisfiedBy(state State) bool {
	if state == "" {
		return false
	}
	if obligation.Finals != "" {
		return obligation.Finals.Contains(state)
	}
	return obligation.Final != "" && obligation.Final == state
}

// Empty reports whether the obligation has no declared final state.
func (obligation Obligation) Empty() bool {
	return obligation.Final == "" && obligation.Finals == ""
}

// FinalStateList returns every state that can discharge this obligation.
func (obligation Obligation) FinalStateList() []State {
	if obligation.Finals != "" {
		return obligation.Finals.States()
	}
	if obligation.Final == "" {
		return nil
	}
	return []State{obligation.Final}
}
