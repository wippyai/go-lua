package contribution

// Delta is the immutable affected-destination projection of one contribution
// successor. A changed producer key marks exactly its output Target once;
// replacing one producer never marks unrelated targets.
type Delta struct {
	base     State
	next     State
	affected []Target
	sealed   bool
}

func newDelta(base, next State, affected []Target) Delta {
	copyOf := append([]Target(nil), affected...)
	result := Delta{base: base, next: next, affected: copyOf, sealed: true}
	return result
}

// Available reports whether the delta retains exact immutable roots and a
// canonical unique affected-destination vector.
func (delta Delta) Available() bool {
	if !delta.sealed || !delta.base.Available() || !delta.next.Available() || !delta.base.Fence().Same(delta.next.Fence()) {
		return false
	}
	if !delta.base.Same(delta.next) && !delta.next.SuccessorOf(delta.base) {
		return false
	}
	for index, target := range delta.affected {
		if !target.Available() || (index > 0 && compareTarget(delta.affected[index-1], target) >= 0) {
			return false
		}
	}
	return true
}

// Base returns the exact predecessor root.
func (delta Delta) Base() State {
	if !delta.Available() {
		return State{}
	}
	return delta.base
}

// Next returns the exact immutable successor root.
func (delta Delta) Next() State {
	if !delta.Available() {
		return State{}
	}
	return delta.next
}

// Changed reports whether this delta changed a producer row.
func (delta Delta) Changed() bool { return delta.Available() && !delta.base.Same(delta.next) }

// AffectedTargets returns the canonical unique output targets touched by this
// exact update. No whole-cell aggregate is exposed and output ports are not
// collapsed merely because their rows coincide.
func (delta Delta) AffectedTargets() []Target {
	if !delta.Available() {
		return nil
	}
	return append([]Target(nil), delta.affected...)
}
