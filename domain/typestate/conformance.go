package typestate

import "fmt"

// AdmitsAcquire reports whether acquiring a resource in state under obligation
// conforms to this state machine: the acquired state is declared, and every
// state that discharges the obligation is declared final. The returned error
// names the first arm the usage violates.
func (d Definition) AdmitsAcquire(state State, obligation Obligation) error {
	if !d.HasState(state) {
		return fmt.Errorf("protocol %q does not declare acquire state %q", d.Protocol, state)
	}
	for _, final := range obligation.FinalStateList() {
		if !d.IsFinal(final) {
			return fmt.Errorf("protocol %q does not declare obligation final state %q", d.Protocol, final)
		}
	}
	return nil
}

// AdmitsTransition reports whether moving a resource from -> to conforms to
// this state machine: both endpoints are declared states and the edge itself is
// declared. The returned error names the first arm the usage violates.
func (d Definition) AdmitsTransition(from, to State) error {
	if !d.HasState(to) {
		return fmt.Errorf("protocol %q does not declare transition target state %q", d.Protocol, to)
	}
	if !d.HasState(from) {
		return fmt.Errorf("protocol %q does not declare transition source state %q", d.Protocol, from)
	}
	if !d.AllowsTransition(from, to) {
		return fmt.Errorf("protocol %q does not declare transition %q -> %q", d.Protocol, from, to)
	}
	return nil
}
