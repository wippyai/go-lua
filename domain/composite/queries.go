package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
)

// queryCells is one pass's per-family payload, indexed by the family's dense
// declaration position in the sealed query surface. The cold pass fills it with
// each family's fragment and the sealed pass with its implementation. A cell is
// produced and consumed only by the contributor that declared the family, so
// this package carries every family's payload without naming one.
type queryCells []query.Cell

// newQueryCells opens one pass's payload over an inventory.
func newQueryCells(entries []*query.Registration) queryCells { return make(queryCells, len(entries)) }

// available states the pass's coverage over the inventory it ran on: every
// declared family carries its cell. A family is answered by its contributor
// alone, so a missing cell is an unanswerable family and never a fallback.
func (cells queryCells) available(entries []*query.Registration) bool {
	if len(cells) != len(entries) {
		return false
	}
	for _, cell := range cells {
		if !cell.Available() {
			return false
		}
	}
	return true
}

// queryPositionForFamily resolves one family's dense declaration position in
// the sealed inventory.
func queryPositionForFamily(state *catalog, family schema.Key) (int, bool) {
	if state == nil || state.sealed == nil {
		return 0, false
	}
	position, declared := state.queryPositions[family]
	return position, declared
}

// subjects opens a subject view over one axis pass's payloads, keyed by the
// axis's authored key. Every bound axis of the pass reaches the view; each
// family is then narrowed to exactly the subjects its own declaration named, so
// a contributor reads no coordinate space its registration omits.
func (cells axisCells) subjects(_ *catalog, entries []*axisTemplate) (query.Subjects, bool) {
	if len(cells) != len(entries)+1 {
		return query.Subjects{}, false
	}
	payloads := make(map[schema.Key]axis.Cell, len(entries))
	for position, entry := range entries {
		cell := cells[position+1]
		if !cell.Available() {
			continue
		}
		payloads[entry.Key()] = cell
	}
	return query.NewSubjects(payloads), true
}

// declareQueries runs the table's cold query pass: every declared family opens
// its own query slot against the axis fragments its subjects produced. It runs
// after the axis pass, because a family declares the read its fold runs over
// against a principal that pass recorded.
//
// The payloads are deliberately opaque here. A family's slot, its result codec,
// and the fold behind them belong to the domain that owns the facts it answers
// from; what this pass owns is that every declared family is asked exactly once
// and in the table's order.
func declareQueries(state *catalog, builder *engine.SchemaBuilder, fragments axisCells) (queryCells, bool) {
	if state == nil || state.sealed == nil {
		return nil, false
	}
	subjects, subjectsOK := fragments.subjects(state, state.axes)
	if !subjectsOK {
		return nil, false
	}
	cells := newQueryCells(state.queries)
	if builder == nil {
		return cells, false
	}
	if len(state.queryContributors) != len(state.queries) {
		return cells, false
	}
	for position, contributor := range state.queryContributors {
		if !contributor.producerComplete() {
			return cells, false
		}
		fragment, ok := contributor.declare(builder, subjects)
		if !ok {
			return cells, false
		}
		cells[position] = fragment
	}
	return cells, cells.available(state.queries)
}

// bindQueries runs the table's hot query pass: every declared family installs
// its typed producer on the bound principal its subject axis produced. Result
// publication is a separate selected-point capability and is not required for
// an observation producer. This runs inside the binding transaction, after
// every rule slot is registered and paired and before the binding becomes
// terminal, which is the one position a query may be bound at.
func bindQueries(state *catalog, binding *engine.SchemaBinding, fragments queryCells, bound axisCells) bool {
	if state == nil || state.sealed == nil || !fragments.available(state.queries) {
		return false
	}
	subjects, subjectsOK := bound.subjects(state, state.axes)
	if !subjectsOK {
		return false
	}
	if len(state.queryContributors) != len(state.queries) {
		return false
	}
	for position, contributor := range state.queryContributors {
		if !contributor.producerComplete() {
			return false
		}
		if !contributor.bind(binding, fragments[position], subjects) {
			return false
		}
	}
	return true
}
