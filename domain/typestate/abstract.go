package typestate

// Abstract is the solved typestate of one resource at one program point: the
// set of declared states it may be in, or Unknown when no set is proven.
//
// It is a lattice element, not a report. Unknown is the top: it is what an
// opaque dispatch, an unproven parameter, or a resource whose identity the
// analysis cannot follow solves to, and it absorbs every join. The empty set is
// the bottom: an unreachable program point solves to it, and no judgment is
// drawn there. Every judgment in this package reads Abstract and never widens
// it, so a consumer cannot turn an unproven state into a proven one by asking a
// different question.
type Abstract struct {
	states  StateSet
	unknown bool
}

// Unknown returns the top element: the resource may be in any state, including
// one this analysis cannot name.
func Unknown() Abstract { return Abstract{unknown: true} }

// Unreachable returns the bottom element: no execution reaches this point, so
// the resource is in no state.
func Unreachable() Abstract { return Abstract{} }

// Exactly returns the solved state of a resource proven to be in one state. An
// empty state name is refused: an unnamed state is not a proof.
func Exactly(state State) (Abstract, bool) {
	set, ok := NewStateSet(state)
	if !ok || set.Empty() {
		return Abstract{}, false
	}
	return Abstract{states: set}, true
}

// Possibly returns the solved state of a resource whose possibilities are the
// members of set. An empty set is the unreachable element, not Unknown.
func Possibly(set StateSet) Abstract { return Abstract{states: set} }

// IsUnknown reports whether the element is top.
func (abstract Abstract) IsUnknown() bool { return abstract.unknown }

// Unreachable reports whether the element is bottom.
func (abstract Abstract) Unreachable() bool { return !abstract.unknown && abstract.states.Empty() }

// States returns the proven possibilities. It is empty for both top and
// bottom, so a caller must consult IsUnknown before reading it as a proof.
func (abstract Abstract) States() StateSet {
	if abstract.unknown {
		return StateSet{}
	}
	return abstract.states
}

// Join is the least upper bound: the element a control-flow merge carries. Top
// absorbs, bottom is neutral, and two proven sets merge to their union.
func (abstract Abstract) Join(other Abstract) Abstract {
	if abstract.unknown || other.unknown {
		return Unknown()
	}
	return Abstract{states: abstract.states.Union(other.states)}
}

// LessOrEqual is the lattice order. It is stated so a rule can prove its own
// monotonicity against this domain rather than against a private convention.
func (abstract Abstract) LessOrEqual(other Abstract) bool {
	if other.unknown {
		return true
	}
	if abstract.unknown {
		return false
	}
	return abstract.states.SubsetOf(other.states)
}

// Proves reports whether the element proves the resource is in state. Only a
// singleton set proves a state: a set that merely contains it leaves another
// possibility live, and top proves nothing.
func (abstract Abstract) Proves(state State) bool {
	return !abstract.unknown && abstract.states.Only(state)
}

// Refutes reports whether the element proves the resource is not in state.
// Top refutes nothing and bottom refutes nothing, because a judgment drawn at
// an unreachable point is not a finding about the program.
func (abstract Abstract) Refutes(state State) bool {
	if abstract.unknown || abstract.states.Empty() || state == "" {
		return false
	}
	return !abstract.states.Contains(state)
}

// Escape applies one declared escape to the solved state. An escape is the
// declaration that an operation hands the resource somewhere this analysis
// does not follow, so every proof about it is discharged and the successor is
// top.
//
// Escape is the only widening in this domain, and it is declared: a resource
// that is merely returned, stored, or passed to a callee whose body is known
// has not escaped, and its obligation survives. Treating those as escapes
// would silence a leak the declaration never released.
func (definition Definition) Escape(_ Abstract) Abstract { return Unknown() }

// Step applies one declared operation to the solved state. A member the edge
// admits moves to the edge's target; a member it does not admit is left in
// place, which over-approximates the unreachable successor of an operation the
// program is not allowed to perform there. The transition itself is judged by
// JudgeTransition; Step never reports, so an invalid operation does not also
// lose the state that made it invalid.
//
// Top steps to top: an unknown state cannot be moved to a known one.
func (definition Definition) Step(solved Abstract, from, to State) Abstract {
	if solved.unknown {
		return Unknown()
	}
	if !definition.AllowsTransition(from, to) {
		return solved
	}
	members := solved.states.List()
	stepped := make([]State, 0, len(members))
	for _, member := range members {
		if member == from {
			stepped = append(stepped, to)
			continue
		}
		stepped = append(stepped, member)
	}
	set, ok := NewStateSet(stepped...)
	if !ok {
		return Unknown()
	}
	return Abstract{states: set}
}
