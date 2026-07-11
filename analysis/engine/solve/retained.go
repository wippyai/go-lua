package solve

import (
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// RetainedBudget bounds logical retained records. Zero fields are unlimited.
// StateRefs counts persistent State values without pretending to measure their
// shared backing stores byte-for-byte.
type RetainedBudget struct {
	MaxOwners    int
	MaxReads     int
	MaxOutputs   int
	MaxStateRefs int
}

type RetainedUsage struct {
	Owners, Reads, Outputs, StateRefs int
}

var ErrRetainedBudget = errors.New("solve: retained generation budget exceeded")

type retainedRead[Cell comparable, State any] struct {
	dependency Cell
	value      State
	revision   uint64
}

type retainedOutput[Cell comparable, State any] struct {
	destination  Cell
	contribution State
}

type retainedOwner[Cell comparable, State any] struct {
	owner   Cell
	reads   []retainedRead[Cell, State]
	outputs []retainedOutput[Cell, State]
}

type retainedReverse[Cell comparable] struct {
	cell   Cell
	owners []Cell
}

type retainedContribution[Cell comparable, State any] struct {
	owner        Cell
	contribution State
}

type retainedDraft[Cell comparable, State any] struct {
	reads       map[Cell]retainedRead[Cell, State]
	outputs     map[Cell]State
	outputOrder []Cell
}

// retainedRecorder is solve-local. A transfer visit is staged and replaces
// the previously committed owner bag only after the callback, cancellation,
// plan-coverage, and budget checks all succeed.
type retainedRecorder[Cell comparable, State any] struct {
	domain    lattice.Lattice[State]
	order     map[Cell]int
	ordered   []Cell
	nextOrder int
	budget    RetainedBudget
	usage     RetainedUsage
	owners    map[Cell]*retainedDraft[Cell, State]
	active    Cell
	staged    *retainedDraft[Cell, State]
}

func newRetainedRecorder[Cell comparable, State any](cells []Cell, domain lattice.Lattice[State], budget RetainedBudget) *retainedRecorder[Cell, State] {
	order := make(map[Cell]int, len(cells))
	for index, cell := range cells {
		order[cell] = index
	}
	return &retainedRecorder[Cell, State]{
		domain: domain, order: order, ordered: append([]Cell(nil), cells...), nextOrder: len(order), budget: budget,
		owners: make(map[Cell]*retainedDraft[Cell, State], len(cells)),
	}
}

func (r *retainedRecorder[Cell, State]) begin(owner Cell) {
	r.active = owner
	r.staged = &retainedDraft[Cell, State]{
		reads:   make(map[Cell]retainedRead[Cell, State]),
		outputs: make(map[Cell]State),
	}
}

func (r *retainedRecorder[Cell, State]) read(dependency Cell, value State, revision uint64) {
	if r.staged == nil {
		return
	}
	r.noteOrder(dependency)
	r.staged.reads[dependency] = retainedRead[Cell, State]{dependency: dependency, value: value, revision: revision}
}

func (r *retainedRecorder[Cell, State]) emit(destination Cell, contribution State) {
	if r.staged == nil {
		return
	}
	r.noteOrder(destination)
	if previous, exists := r.staged.outputs[destination]; exists {
		r.staged.outputs[destination] = r.domain.Join(previous, contribution)
		return
	}
	r.staged.outputs[destination] = contribution
	r.staged.outputOrder = append(r.staged.outputOrder, destination)
}

func (r *retainedRecorder[Cell, State]) noteOrder(cell Cell) {
	if _, exists := r.order[cell]; exists {
		return
	}
	r.order[cell] = r.nextOrder
	r.nextOrder++
	r.ordered = append(r.ordered, cell)
}

func (r *retainedRecorder[Cell, State]) discard() {
	r.staged = nil
	var zero Cell
	r.active = zero
}

func (r *retainedRecorder[Cell, State]) commit() error {
	if r.staged == nil {
		return nil
	}
	next := r.usage
	if previous := r.owners[r.active]; previous != nil {
		next.Reads -= len(previous.reads)
		next.Outputs -= len(previous.outputs)
		next.StateRefs -= len(previous.reads) + len(previous.outputs)
	} else {
		next.Owners++
	}
	next.Reads += len(r.staged.reads)
	next.Outputs += len(r.staged.outputs)
	next.StateRefs += len(r.staged.reads) + len(r.staged.outputs)
	if err := retainedBudgetError(r.budget, next); err != nil {
		return err
	}
	r.owners[r.active] = r.staged
	r.usage = next
	r.discard()
	return nil
}

func retainedBudgetError(budget RetainedBudget, usage RetainedUsage) error {
	exceeds := func(limit, used int) bool { return limit > 0 && used > limit }
	if !exceeds(budget.MaxOwners, usage.Owners) && !exceeds(budget.MaxReads, usage.Reads) &&
		!exceeds(budget.MaxOutputs, usage.Outputs) && !exceeds(budget.MaxStateRefs, usage.StateRefs) {
		return nil
	}
	return fmt.Errorf("%w: usage=%+v budget=%+v", ErrRetainedBudget, usage, budget)
}

func (r *retainedRecorder[Cell, State]) compact(cells []Cell) ([]retainedOwner[Cell, State], []retainedReverse[Cell], []retainedReverse[Cell]) {
	owners := make([]retainedOwner[Cell, State], 0, len(r.owners))
	readers := make(map[Cell][]Cell)
	outputs := make(map[Cell][]Cell)
	for _, owner := range cells {
		draft := r.owners[owner]
		if draft == nil {
			continue
		}
		item := retainedOwner[Cell, State]{owner: owner}
		dependencies := make([]Cell, 0, len(draft.reads))
		for dependency := range draft.reads {
			dependencies = append(dependencies, dependency)
		}
		sort.Slice(dependencies, func(i, j int) bool { return r.order[dependencies[i]] < r.order[dependencies[j]] })
		for _, dependency := range dependencies {
			item.reads = append(item.reads, draft.reads[dependency])
			readers[dependency] = append(readers[dependency], owner)
		}
		for _, destination := range draft.outputOrder {
			item.outputs = append(item.outputs, retainedOutput[Cell, State]{destination: destination, contribution: draft.outputs[destination]})
			outputs[destination] = append(outputs[destination], owner)
		}
		owners = append(owners, item)
	}
	return owners, compactRetainedReverse(r.ordered, readers), compactRetainedReverse(r.ordered, outputs)
}

func compactRetainedReverse[Cell comparable](cells []Cell, values map[Cell][]Cell) []retainedReverse[Cell] {
	out := make([]retainedReverse[Cell], 0, len(values))
	for _, cell := range cells {
		if owners := values[cell]; len(owners) != 0 {
			out = append(out, retainedReverse[Cell]{cell: cell, owners: append([]Cell(nil), owners...)})
		}
	}
	return out
}
