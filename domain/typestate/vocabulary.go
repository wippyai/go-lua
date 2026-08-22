package typestate

// Protocol identifies one declared state-machine namespace.
type Protocol string

// State identifies one declared protocol state.
type State string

// Obligation is the set of states that can discharge one lifecycle obligation.
// It is a named role of the canonical StateSet rather than a second set
// encoding, so an obligation and a solved state are compared as the same kind
// of object. Its member is private so callers cannot keep a legacy scalar arm
// beside the set or inject a non-canonical spelling.
type Obligation struct{ states StateSet }

// NewObligation issues one deterministic obligation. The empty set is a valid
// absence of obligation; an empty state member is malformed and refused.
func NewObligation(states ...State) (Obligation, bool) {
	set, ok := NewStateSet(states...)
	if !ok {
		return Obligation{}, false
	}
	return Obligation{states: set}, true
}

// FinalStateList returns every state that can discharge this obligation in
// canonical order.
func (obligation Obligation) FinalStateList() []State { return obligation.states.List() }

// FinalStates returns the discharging states as the canonical set.
func (obligation Obligation) FinalStates() StateSet { return obligation.states }

// SatisfiedBy reports whether state is one of the satisfying states.
func (obligation Obligation) SatisfiedBy(state State) bool { return obligation.states.Contains(state) }

// Empty reports whether the obligation has no declared final state.
func (obligation Obligation) Empty() bool { return obligation.states.Empty() }
