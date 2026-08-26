package contribution

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// State is one immutable contribution root. Unchanged rows are copied by
// value into a successor slice; no caller-owned row or slice is retained.
type State struct {
	root *stateRoot
}

type stateRoot struct {
	parent    *stateRoot
	fence     binding.Fence
	namespace Handle
	rows      []Row
	sealed    bool
}

// New creates an empty contribution root for one exact runtime fence. It does
// not issue an invocation handle.
func New(fence binding.Fence) (State, bool) {
	if !fence.Available() {
		return State{}, false
	}
	result := State{root: &stateRoot{fence: fence, rows: make([]Row, 0), sealed: true}}
	return result, result.Available()
}

// Available reports whether the root retains a complete fence and sealed row
// vector. A successful root remains immutable after construction.
func (state State) Available() bool {
	return state.root != nil && state.root.sealed && state.root.fence.Available() && state.root.rows != nil && validRows(state.root.rows, state.root.fence, state.root.namespace)
}

func validRows(rows []Row, fence binding.Fence, namespace Handle) bool {
	for index, row := range rows {
		if !row.ValidFor(fence) {
			return false
		}
		if namespace.Available() && !row.Key.Invocation.SameDirectory(namespace) {
			return false
		}
		if index > 0 && compareKey(rows[index-1].Key, row.Key) >= 0 {
			return false
		}
	}
	return true
}

// Fence returns the exact runtime authority captured by the root.
func (state State) Fence() binding.Fence {
	if !state.Available() {
		return binding.Fence{}
	}
	return state.root.fence
}

// Same reports exact immutable root identity.
func (state State) Same(other State) bool {
	return state.Available() && other.Available() && state.root == other.root
}

// SuccessorOf proves direct immutable ancestry.
func (state State) SuccessorOf(base State) bool {
	return state.Available() && base.Available() && !state.Same(base) && state.root.parent == base.root && state.Fence().Same(base.Fence())
}

// Len reports the number of producer rows, not the number of destination
// cells. Multiple producers at one destination therefore remain visible.
func (state State) Len() int {
	if !state.Available() {
		return 0
	}
	return len(state.root.rows)
}

// Rows returns every contribution row in canonical deterministic key order.
func (state State) Rows() []Row {
	if !state.Available() {
		return nil
	}
	return append([]Row(nil), state.root.rows...)
}

// Row resolves one exact producer key.
func (state State) Row(key Key) (Row, bool) {
	if !state.Available() || !key.ValidFor(state.root.fence) || state.root.namespace.Available() && !key.Invocation.SameDirectory(state.root.namespace) {
		return Row{}, false
	}
	for _, row := range state.root.rows {
		if sameKey(row.Key, key) {
			return row, true
		}
	}
	return Row{}, false
}

// RowsFor returns all producer rows for one destination in canonical key
// order. It never collapses those rows into one whole-cell value.
func (state State) RowsFor(target Target) []Row {
	if !state.Available() || !target.Available() {
		return nil
	}
	result := make([]Row, 0)
	for _, row := range state.root.rows {
		if row.Target().Same(target) {
			result = append(result, row)
		}
	}
	return result
}

// Targets returns the sorted unique output targets represented by the current
// contribution root.  It retains output-port identity, so two columns on the
// same row are never merged into one reduction cell.
func (state State) Targets() []Target {
	if !state.Available() {
		return nil
	}
	return targetsForRows(state.root.rows)
}

// Upsert atomically inserts or replaces exactly one producer row. The old row
// is not observable in the returned successor, and sibling producer rows at
// the same destination remain untouched.
func (state State) Upsert(row Row) (State, Delta, bool) {
	if !state.Available() || !row.ValidFor(state.root.fence) || state.root.namespace.Available() && !row.Key.Invocation.SameDirectory(state.root.namespace) {
		return State{}, Delta{}, false
	}
	rows := append([]Row(nil), state.root.rows...)
	position := -1
	for index, prior := range rows {
		if sameKey(prior.Key, row.Key) {
			position = index
			break
		}
	}
	if position >= 0 && samePayload(rows[position], row) {
		delta := newDelta(state, state, nil)
		return state, delta, delta.Available()
	}
	if position >= 0 {
		rows[position] = row
	} else {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool { return compareKey(rows[left].Key, rows[right].Key) < 0 })
	namespace := state.root.namespace
	if !namespace.Available() {
		namespace = row.Key.Invocation
	}
	next := State{root: &stateRoot{parent: state.root, fence: state.root.fence, namespace: namespace, rows: rows, sealed: true}}
	if !next.Available() {
		return State{}, Delta{}, false
	}
	delta := newDelta(state, next, []Target{row.Target()})
	if !delta.Available() {
		return State{}, Delta{}, false
	}
	return next, delta, true
}

// Remove atomically deletes exactly one producer row by its full key. It does
// not remove a destination cell or any sibling producer contribution.
func (state State) Remove(key Key) (State, Delta, bool) {
	if !state.Available() || !key.ValidFor(state.root.fence) || state.root.namespace.Available() && !key.Invocation.SameDirectory(state.root.namespace) {
		return State{}, Delta{}, false
	}
	position := -1
	for index, row := range state.root.rows {
		if sameKey(row.Key, key) {
			position = index
			break
		}
	}
	if position < 0 {
		delta := newDelta(state, state, nil)
		return state, delta, delta.Available()
	}
	rows := make([]Row, 0, len(state.root.rows)-1)
	rows = append(rows, state.root.rows[:position]...)
	rows = append(rows, state.root.rows[position+1:]...)
	next := State{root: &stateRoot{parent: state.root, fence: state.root.fence, namespace: state.root.namespace, rows: rows, sealed: true}}
	if !next.Available() {
		return State{}, Delta{}, false
	}
	delta := newDelta(state, next, []Target{{Port: key.Port, Destination: key.Destination}})
	if !delta.Available() {
		return State{}, Delta{}, false
	}
	return next, delta, true
}

// RemoveRow is the row-shaped spelling of Remove. The key, rather than the
// payload, authorizes removal; this keeps removal producer-specific while
// allowing callers that already hold a row to avoid reconstructing its key.
func (state State) RemoveRow(row Row) (State, Delta, bool) {
	if !state.Available() || !row.ValidFor(state.root.fence) {
		return State{}, Delta{}, false
	}
	return state.Remove(row.Key)
}

func targetsForRows(rows []Row) []Target {
	if rows == nil {
		return nil
	}
	result := make([]Target, 0)
	for _, row := range rows {
		target := row.Target()
		if len(result) == 0 || !result[len(result)-1].Same(target) {
			result = append(result, target)
		}
	}
	// Rows are key-sorted by destination and port, so this is already
	// canonical. Keep the explicit sort to make the helper safe for future
	// projections.
	sort.Slice(result, func(left, right int) bool { return compareTarget(result[left], result[right]) < 0 })
	return result
}
