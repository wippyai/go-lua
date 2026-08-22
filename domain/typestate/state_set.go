package typestate

import (
	"strconv"
	"strings"
)

// StateSet is the canonical comparable set of declared protocol states. It is
// the one set representation this domain has: the declared obligation a
// lifecycle label is discharged against and the solved state an analysis
// carries are the same set object playing two roles, so neither role owns a
// private spelling of it.
//
// The representation is a length-prefixed canonical encoding of the members in
// sorted order. Two sets built from the same members in any order are therefore
// equal under ==, which is what lets a set be a map key, a lattice element, and
// a field of a comparable label at once.
type StateSet struct{ encoded string }

// NewStateSet issues one deterministic set. The empty set is valid; an empty
// state member is malformed and refused, because a state with no name cannot be
// compared against a declared one.
func NewStateSet(states ...State) (StateSet, bool) {
	if len(states) == 0 {
		return StateSet{}, true
	}
	for _, state := range states {
		if state == "" {
			return StateSet{}, false
		}
	}
	unique := uniqueStates(states)
	var builder strings.Builder
	for _, state := range unique {
		raw := state.String()
		builder.WriteString(strconv.Itoa(len(raw)))
		builder.WriteByte(':')
		builder.WriteString(raw)
	}
	return StateSet{encoded: builder.String()}, true
}

// List returns every member in canonical order.
func (set StateSet) List() []State {
	if set.encoded == "" {
		return nil
	}
	raw := set.encoded
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

// Len reports the number of members.
func (set StateSet) Len() int { return len(set.List()) }

// Empty reports whether the set has no member.
func (set StateSet) Empty() bool { return set.encoded == "" }

// Contains reports whether state is a member. The empty state is never a
// member, so a caller that lost a state name cannot accidentally match.
func (set StateSet) Contains(state State) bool {
	if state == "" || set.encoded == "" {
		return false
	}
	for _, candidate := range set.List() {
		if candidate == state {
			return true
		}
	}
	return false
}

// Only reports whether state is the set's sole member. It is the exact test a
// proof of one state needs: a set that merely contains the state does not prove
// the resource is in it.
func (set StateSet) Only(state State) bool {
	members := set.List()
	return len(members) == 1 && state != "" && members[0] == state
}

// Union returns the least set containing both operands. It is the join of the
// state lattice: two control-flow predecessors that solve different states
// reach their successor with both possibilities live.
func (set StateSet) Union(other StateSet) StateSet {
	switch {
	case set.encoded == other.encoded:
		return set
	case set.encoded == "":
		return other
	case other.encoded == "":
		return set
	}
	merged, ok := NewStateSet(append(set.List(), other.List()...)...)
	if !ok {
		return StateSet{}
	}
	return merged
}

// SubsetOf reports whether every member of set is a member of other. It is the
// order of the state lattice, so a caller can state monotonicity directly.
func (set StateSet) SubsetOf(other StateSet) bool {
	if set.encoded == other.encoded {
		return true
	}
	for _, member := range set.List() {
		if !other.Contains(member) {
			return false
		}
	}
	return true
}
