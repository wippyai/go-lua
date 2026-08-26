package contribution

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
)

// ApplyTransitions applies one nonempty ordered contribution vector against
// one pair of immutable roots.  The directory and state are published only
// after every Before side has matched the current working row and every After
// side has been validated.  A caller therefore receives either no result or
// one direct directory successor, one direct state successor, and one delta
// rooted at the exact input state.
//
// Ordering is semantic: a later transition may consume the row produced by an
// earlier transition, but a stale or contradictory Before refuses the whole
// vector.  No aggregate is reduced here; only exact producer rows are
// inserted, replaced, or removed.
func ApplyTransitions(baseDirectory Directory, baseState State, transitions []invocation.ContributionTransition) (Directory, State, Delta, bool) {
	if !baseDirectory.Available() || !baseState.Available() || len(transitions) == 0 || !baseDirectory.Fence().Same(baseState.Fence()) {
		return Directory{}, State{}, Delta{}, false
	}
	if !baseStateRowsBelongTo(baseDirectory, baseState) {
		return Directory{}, State{}, Delta{}, false
	}

	addresses := make([]invocation.InvocationAddress, len(transitions))
	for index, transition := range transitions {
		if !transition.ValidFor(baseDirectory.Fence()) {
			return Directory{}, State{}, Delta{}, false
		}
		address := transition.Address()
		if !address.ValidFor(baseDirectory.Fence()) {
			return Directory{}, State{}, Delta{}, false
		}
		addresses[index] = address
	}

	nextDirectory, handles, ok := baseDirectory.internBatch(addresses)
	if !ok {
		return Directory{}, State{}, Delta{}, false
	}

	working := baseState.Rows()
	affected := make([]Target, 0, len(transitions))
	for index, transition := range transitions {
		cell := transition.Destination()
		if !cell.Available() || !cell.ValidFor(baseDirectory.Fence()) {
			return Directory{}, State{}, Delta{}, false
		}
		port := transition.Port()
		if !port.Available() || cell.Column() != port.Column || !transition.Spec().Available() || transition.Spec().Port() != port {
			return Directory{}, State{}, Delta{}, false
		}
		key, keyOK := NewKey(handles[index], port, cell.Row())
		if !keyOK {
			return Directory{}, State{}, Delta{}, false
		}
		before, beforePresent := transition.Before()
		after, afterPresent := transition.After()
		if !beforePresent && !afterPresent {
			return Directory{}, State{}, Delta{}, false
		}
		position := rowPosition(working, key)
		if beforePresent {
			expected, expectedOK := rowFromSide(key, cell, before)
			if !expectedOK || position < 0 || !sameRow(working[position], expected) {
				return Directory{}, State{}, Delta{}, false
			}
		}

		if afterPresent {
			replacement, replacementOK := rowFromSide(key, cell, after)
			if !replacementOK {
				return Directory{}, State{}, Delta{}, false
			}
			if position >= 0 {
				// An After-only transport is the normal reevaluation form:
				// it inserts when absent and replaces when present.  Exact
				// equality is an authenticated no-op, not a refusal.
				if !sameRow(working[position], replacement) {
					working[position] = replacement
					markTarget(&affected, Target{Port: port, Destination: cell.Row()})
				}
			} else {
				working = append(working, replacement)
				markTarget(&affected, Target{Port: port, Destination: cell.Row()})
			}
		} else {
			if position < 0 {
				return Directory{}, State{}, Delta{}, false
			}
			working = append(working[:position], working[position+1:]...)
		}
		if !afterPresent || position < 0 {
			// Deletions are marked here; After-present changes were marked
			// at the exact replacement/insertion point above.
			markTarget(&affected, Target{Port: port, Destination: cell.Row()})
		}
	}

	if rowsEqual(working, baseState.Rows()) {
		// Re-evaluation may publish an exact same After-only value.  Keep
		// the input roots and expose a valid empty delta; no authority churn
		// or directory successor is manufactured for a semantic no-op.
		empty := newDelta(baseState, baseState, nil)
		if !empty.Available() {
			return Directory{}, State{}, Delta{}, false
		}
		return baseDirectory, baseState, empty, true
	}
	sort.Slice(working, func(left, right int) bool { return compareKey(working[left].Key, working[right].Key) < 0 })
	sort.Slice(affected, func(left, right int) bool { return compareTarget(affected[left], affected[right]) < 0 })
	if !uniqueTargets(affected) {
		return Directory{}, State{}, Delta{}, false
	}

	nextState, ok := stateSuccessor(baseState, nextDirectory, working)
	if !ok || !nextState.SuccessorOf(baseState) || (!nextDirectory.Same(baseDirectory) && !nextDirectory.SuccessorOf(baseDirectory)) {
		return Directory{}, State{}, Delta{}, false
	}
	delta := newDelta(baseState, nextState, affected)
	if !delta.Available() {
		return Directory{}, State{}, Delta{}, false
	}
	return nextDirectory, nextState, delta, true
}

func baseStateRowsBelongTo(directory Directory, state State) bool {
	for _, row := range state.Rows() {
		if !row.ValidFor(state.Fence()) || !directory.Contains(row.Key.Invocation) {
			return false
		}
	}
	return true
}

func stateSuccessor(base State, directory Directory, rows []Row) (State, bool) {
	if !base.Available() || !directory.Available() || !base.Fence().Same(directory.Fence()) || rows == nil {
		return State{}, false
	}
	namespace := base.root.namespace
	if !namespace.Available() && len(rows) != 0 {
		namespace = rows[0].Key.Invocation
	}
	if !namespace.Available() && len(rows) != 0 {
		return State{}, false
	}
	if !validRows(rows, base.Fence(), namespace) {
		return State{}, false
	}
	for _, row := range rows {
		if !directory.Contains(row.Key.Invocation) {
			return State{}, false
		}
	}
	// Preserve a non-nil empty row vector. State.Available deliberately treats
	// nil as an invalid/unsealed representation, so deleting the final
	// producer must still produce a valid empty successor root.
	ownedRows := make([]Row, len(rows))
	copy(ownedRows, rows)
	next := State{root: &stateRoot{parent: base.root, fence: base.root.fence, namespace: namespace, rows: ownedRows, sealed: true}}
	return next, next.Available()
}

func rowPosition(rows []Row, key Key) int {
	for index, row := range rows {
		if sameKey(row.Key, key) {
			return index
		}
	}
	return -1
}

func rowFromSide(key Key, cell binding.CellToken, side binding.ContributionSide) (Row, bool) {
	if !side.Present() {
		return Row{}, false
	}
	return NewRow(key, cell, side.Value(), side.Presence(), side.Lineage())
}

func sameRow(left, right Row) bool {
	return sameKey(left.Key, right.Key) && left.Destination.Same(right.Destination) && samePayload(left, right)
}

func rowsEqual(left, right []Row) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !sameRow(left[index], right[index]) {
			return false
		}
	}
	return true
}

func markTarget(targets *[]Target, candidate Target) {
	for _, prior := range *targets {
		if prior.Same(candidate) {
			return
		}
	}
	*targets = append(*targets, candidate)
}

func uniqueTargets(targets []Target) bool {
	for index := 1; index < len(targets); index++ {
		if targets[index-1].Same(targets[index]) {
			return false
		}
	}
	return true
}
