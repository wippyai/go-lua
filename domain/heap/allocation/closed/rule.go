// Package closed owns Heap's one atomic fixed-scalar table-constructor Rule.
// Link/source owns constructor topology and causal same-read witnesses; Value
// owns atoms; keymatch owns atom-to-key projection; Heap owns the complete
// object/world construction.  This package introduces no field Fact plane.
package closed

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// resultClosed is the heap/reducer/closed fold: it concludes the one Heap
// world the sealed scalar constructor denotes, and nothing else. Its whole
// answer is a lattice value paired with the sealed outcome that value is
// delivered under - it schedules nothing, locates nothing, and publishes
// nothing.
func (judgment Judgment) resultClosed(root heapdomain.Key, predecessor heapdomain.Value, cells execution.SummaryVector[valuedomain.Value]) (heapdomain.Value, structure.ReductionOutcome) {
	if !judgment.Valid() {
		return heapdomain.Value{}, structure.Refuse
	}
	schema, values, projection := judgment.heaps, judgment.values, judgment.projection
	// What a constructor consists of is resolved from the schemas this
	// judgment was sealed with, at the coordinate the candidate names. The
	// descriptor is not carried beside the row: it is a function of the two
	// schemas, and a copy travelling with the candidate would be a second
	// answer that goes stale on its own.
	operand, operandOK := source.NewClosed(schema, values, root)
	if !operandOK {
		return heapdomain.Value{}, structure.Refuse
	}
	if !operand.FencedTo(schema, values) || cells.Count() != operand.CoordinateCount() {
		return heapdomain.Value{}, structure.Refuse
	}
	inputs := make([]valuedomain.Value, cells.Count())
	for index := range inputs {
		value, present, available := cells.At(index)
		if !available {
			return heapdomain.Value{}, structure.Refuse
		}
		if !present {
			return heapdomain.Value{}, structure.NoCandidate
		}
		inputs[index] = value
	}
	return evaluateClosed(schema, values, projection, operand, predecessor, inputs)
}

// evaluateClosed enumerates the sealed finite atom product without Go
// recursion.  Every coordinate is picked once and reused at all of its source
// uses; source.Closed has already collapsed only Link-proved direct same-read
// pairs to one ordinal.  Distinct ordinals remain independent by construction.
//
// Two quotients keep that product finite without a budget or a cap.  A
// coordinate read exactly once, in the stored-value role only, selects no key
// and correlates no second cell, so its alternatives fold through the
// value-level projection into one CellState instead of one World per atom.
// Every coordinate that still enumerates is first quotiented by its
// heap-observable class: alternatives that agree on that class produce
// identical worlds, which the world accumulator was already discarding.
func evaluateClosed(schema heapdomain.Schema, values *valuedomain.Schema, projection *keymatch.SelectorProjection, operand source.Closed, predecessor heapdomain.Value, inputs []valuedomain.Value) (heapdomain.Value, structure.ReductionOutcome) {
	if values == nil || projection == nil || len(inputs) != operand.CoordinateCount() || !schema.Admits(operand.Key(), predecessor) {
		return heapdomain.Value{}, structure.Refuse
	}
	fields, fieldsOK := fieldsFor(operand)
	if !fieldsOK {
		return heapdomain.Value{}, structure.Refuse
	}
	folded, foldedOK := payloadOnlyCoordinates(fields, len(inputs))
	if !foldedOK {
		return heapdomain.Value{}, structure.Refuse
	}
	choices, payloads, choicesOK := coordinateChoices(projection, inputs, folded)
	order, orderOK := coordinateOrder(fields, len(inputs), folded)
	none, noneOK := schema.ContainmentNone()
	base, baseOK := schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !choicesOK || !orderOK || !noneOK || !baseOK {
		return heapdomain.Value{}, structure.Refuse
	}

	state := &application{
		schema:   schema,
		values:   values,
		fields:   fields,
		folded:   folded,
		payloads: payloads,
		bindings: make([]valuedomain.Atom, len(inputs)),
		bound:    make([]bool, len(inputs)),
	}
	var worlds []heapdomain.Value
	var occupied []bool
	complete := func(init heapdomain.ObjectInit) bool {
		fresh, freshOK := init.Finish()
		created, createdOK := schema.Create(predecessor, operand.Key(), fresh)
		return freshOK && createdOK && accumulateWorld(&worlds, &occupied, created)
	}

	// An operand whose every coordinate folds has one complete leaf: there is
	// nothing left to branch on, and the single object already carries each
	// coordinate's whole disjunction.
	if len(order) == 0 {
		child := base
		nextField, branchNormal, applied := state.applyReady(0, &child)
		if !applied {
			return heapdomain.Value{}, structure.Refuse
		}
		if !branchNormal {
			return finishWorlds(worlds, occupied)
		}
		if nextField != len(fields) || !complete(child) {
			return heapdomain.Value{}, structure.Refuse
		}
		return finishWorlds(worlds, occupied)
	}

	// A child frame owns the coordinate selected by its parent.  Its ObjectInit
	// is a private copy-fork of the immutable unpublished prefix.  Thus every
	// leaf finishes exactly one whole object; no leaf ever extends a sibling.
	type frame struct {
		depth   int
		next    int
		field   int
		init    heapdomain.ObjectInit
		release int
	}
	stack := []frame{{depth: 0, field: 0, init: base, release: -1}}
	pop := func() {
		last := len(stack) - 1
		release := stack[last].release
		stack = stack[:last]
		if release >= 0 {
			state.bound[release] = false
			state.bindings[release] = valuedomain.Atom{}
		}
	}

	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if current.depth < 0 || current.depth >= len(order) {
			return heapdomain.Value{}, structure.Refuse
		}
		coordinate := order[current.depth]
		if coordinate < 0 || coordinate >= len(choices) || current.next >= len(choices[coordinate]) {
			pop()
			continue
		}
		atom := choices[coordinate][current.next]
		current.next++
		if state.bound[coordinate] {
			return heapdomain.Value{}, structure.Refuse
		}
		state.bindings[coordinate], state.bound[coordinate] = atom, true
		child := current.init
		nextField, branchNormal, applied := state.applyReady(current.field, &child)
		if !applied {
			state.bound[coordinate], state.bindings[coordinate] = false, valuedomain.Atom{}
			return heapdomain.Value{}, structure.Refuse
		}
		if !branchNormal {
			state.bound[coordinate], state.bindings[coordinate] = false, valuedomain.Atom{}
			continue
		}
		if current.depth+1 != len(order) {
			stack = append(stack, frame{depth: current.depth + 1, field: nextField, init: child, release: coordinate})
			continue
		}
		completed := nextField == len(fields) && complete(child)
		state.bound[coordinate], state.bindings[coordinate] = false, valuedomain.Atom{}
		if !completed {
			return heapdomain.Value{}, structure.Refuse
		}
	}
	return finishWorlds(worlds, occupied)
}

// accumulateWorld stores a binary-carry join forest. A left fold repeatedly
// copies and rescans every already-produced complete World; equal-sized joins
// preserve the exact same lattice result while bounding each leaf's merge
// depth logarithmically. Slots are indexed by represented leaf count, with no
// map, budget, or cardinality policy participating in the semantics.
func accumulateWorld(slots *[]heapdomain.Value, occupied *[]bool, leaf heapdomain.Value) bool {
	if slots == nil || occupied == nil {
		return false
	}
	for level := 0; ; level++ {
		if level == len(*slots) {
			*slots = append(*slots, heapdomain.Value{})
			*occupied = append(*occupied, false)
		}
		if !(*occupied)[level] {
			(*slots)[level], (*occupied)[level] = leaf, true
			return true
		}
		joined, joinedOK := heapdomain.Join((*slots)[level], leaf)
		if !joinedOK {
			return false
		}
		(*slots)[level], (*occupied)[level] = heapdomain.Value{}, false
		leaf = joined
	}
}

func finishWorlds(slots []heapdomain.Value, occupied []bool) (heapdomain.Value, structure.ReductionOutcome) {
	if len(slots) != len(occupied) {
		return heapdomain.Value{}, structure.Refuse
	}
	var result heapdomain.Value
	have := false
	for level := len(slots) - 1; level >= 0; level-- {
		if !occupied[level] {
			continue
		}
		if !have {
			result, have = slots[level], true
			continue
		}
		joined, joinedOK := heapdomain.Join(result, slots[level])
		if !joinedOK {
			return heapdomain.Value{}, structure.Refuse
		}
		result = joined
	}
	if !have {
		return heapdomain.Value{}, structure.NoCandidate
	}
	return result, structure.Concrete
}

func fieldsFor(operand source.Closed) ([]source.Field, bool) {
	count := operand.Count()
	if count == 0 {
		return nil, false
	}
	fields := make([]source.Field, count)
	for index := range fields {
		field, ok := operand.At(index)
		if !ok || field.Ordinal() != uint32(index+1) {
			return nil, false
		}
		fields[index] = field
	}
	return fields, true
}

// payloadOnlyCoordinates marks every coordinate this constructor reads exactly
// once, in the stored-value role only.  Such a coordinate contributes the same
// cell at the same selector in every world its alternatives would fork, so the
// disjunction belongs inside that one cell.  A coordinate that also selects a
// key, or that a second source use reads, carries a real correlation across
// cells and keeps its own enumerated world.  The use counts are read off the
// descriptor's existing ordinal binding; this introduces no second vocabulary.
func payloadOnlyCoordinates(fields []source.Field, count int) ([]bool, bool) {
	if count <= 0 || len(fields) == 0 {
		return nil, false
	}
	uses := make([]int, count)
	keys := make([]bool, count)
	for _, field := range fields {
		value := field.ValueOrdinal()
		if uint64(value) >= uint64(count) {
			return nil, false
		}
		uses[value]++
		key, dynamic := field.DynamicKeyOrdinal()
		if !dynamic {
			continue
		}
		if uint64(key) >= uint64(count) {
			return nil, false
		}
		uses[key]++
		keys[key] = true
	}
	folded := make([]bool, count)
	for index := range folded {
		folded[index] = uses[index] == 1 && !keys[index]
	}
	return folded, true
}

// coordinateChoices quotients each coordinate's alternatives by exactly what
// Heap construction observes.  It intentionally goes through the sealed
// SelectorProjection, whose walk is Value.VisitSupport: Top includes every
// sealed atom, including opaque/reference alternatives that a kind-only scan
// would silently drop.  Its canonical iteration order drives deterministic
// enumeration; no Go map determines semantic output order.  Enumerated
// coordinates receive the full class quotient, folded coordinates the coarser
// stored-value quotient that ignores key selection.
func coordinateChoices(projection *keymatch.SelectorProjection, inputs []valuedomain.Value, folded []bool) (choices, payloads [][]valuedomain.Atom, ok bool) {
	if projection == nil || len(folded) != len(inputs) {
		return nil, nil, false
	}
	choices = make([][]valuedomain.Atom, len(inputs))
	payloads = make([][]valuedomain.Atom, len(inputs))
	for index, input := range inputs {
		if input.IsBottom() {
			return nil, nil, false
		}
		target := &choices[index]
		visit := projection.VisitClasses
		if folded[index] {
			target = &payloads[index]
			visit = projection.VisitPayloadClasses
		}
		if !visit(input, func(atom valuedomain.Atom) bool {
			*target = append(*target, atom)
			return true
		}) || len(*target) == 0 {
			return nil, nil, false
		}
	}
	return choices, payloads, true
}

// coordinateOrder is first source use (dynamic key then payload) with the
// descriptor's ordinal mapping retained for access to the canonical summary
// input.  It is an execution order, never a second coordinate vocabulary.
// Folded coordinates are deliberately absent: they are not branched on, while
// still having to be reached by some source use for the operand to be whole.
func coordinateOrder(fields []source.Field, count int, folded []bool) ([]int, bool) {
	if count <= 0 || len(folded) != count {
		return nil, false
	}
	want := 0
	for _, skip := range folded {
		if !skip {
			want++
		}
	}
	seen := make([]bool, count)
	order := make([]int, 0, want)
	reached := 0
	appendOrdinal := func(ordinal uint32) bool {
		if uint64(ordinal) >= uint64(count) {
			return false
		}
		index := int(ordinal)
		if seen[index] {
			return true
		}
		seen[index] = true
		reached++
		if !folded[index] {
			order = append(order, index)
		}
		return true
	}
	for _, field := range fields {
		if key, dynamic := field.DynamicKeyOrdinal(); dynamic && !appendOrdinal(key) {
			return nil, false
		}
		if !appendOrdinal(field.ValueOrdinal()) {
			return nil, false
		}
	}
	return order, len(order) == want && reached == count
}

// application is one evaluation's immutable construction context together with
// its current coordinate selection.  It exists so the construction rules read
// the same descriptor, quotient, and binding vector at every field.
type application struct {
	schema   heapdomain.Schema
	values   *valuedomain.Schema
	fields   []source.Field
	folded   []bool
	payloads [][]valuedomain.Atom
	bindings []valuedomain.Atom
	bound    []bool
}

// applyReady extends exactly the source-order prefix whose inputs are ready.
// The first field that is not ready is a hard frontier: later fields cannot
// move ahead of it, even when their coordinates happen to be selected already.
// A folded coordinate is always ready; a field still waits for its own dynamic
// key, which is never folded.
func (state *application) applyReady(start int, init *heapdomain.ObjectInit) (next int, normal, ok bool) {
	if state == nil || state.values == nil || init == nil || start < 0 || start > len(state.fields) ||
		len(state.bindings) != len(state.bound) || len(state.folded) != len(state.bound) {
		return 0, false, false
	}
	for next = start; next < len(state.fields); next++ {
		field := state.fields[next]
		valueOrdinal := field.ValueOrdinal()
		if uint64(valueOrdinal) >= uint64(len(state.bound)) {
			return 0, false, false
		}
		if key, dynamic := field.DynamicKeyOrdinal(); dynamic {
			if uint64(key) >= uint64(len(state.bound)) {
				return 0, false, false
			}
			if !state.bound[key] {
				return next, true, true
			}
		}
		if !state.folded[valueOrdinal] && !state.bound[valueOrdinal] {
			return next, true, true
		}
		selector, keyContainment, selected, selectedOK := state.selectorFor(field)
		if !selectedOK {
			return 0, false, false
		}
		if !selected {
			return next, false, true
		}
		cell, cellOK := state.cellFor(field, keyContainment)
		if !cellOK || !init.Apply(selector, cell) {
			return 0, false, false
		}
	}
	return next, true, true
}

// selectorFor consumes a field's static typed selector or the single atom
// selected for its dynamic key coordinate.  A definitely-invalid key has no
// normal successor.  An atom that may be valid (including opaque numbers)
// retains the valid heap branch; its error branch is not encoded as Heap.
func (state *application) selectorFor(field source.Field) (heapdomain.KeySelector, heapdomain.Containment, bool, bool) {
	if selector, exact := field.ExactSelector(); exact {
		none, ok := state.schema.ContainmentNone()
		return selector, none, true, ok
	}
	ordinal, dynamic := field.DynamicKeyOrdinal()
	if !dynamic || uint64(ordinal) >= uint64(len(state.bindings)) || !state.bound[ordinal] {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false, false
	}
	atom := state.bindings[ordinal]
	if !atom.TableKeyValidity().MayBeValid() {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false, true
	}
	alternative, ok := keymatch.Project(state.schema, state.values, atom)
	if !ok {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false, false
	}
	return alternative.Selector(), alternative.Containment(), true, true
}

// cellFor issues the one complete cell a field stores under its selected key.
func (state *application) cellFor(field source.Field, keyContainment heapdomain.Containment) (heapdomain.CellState, bool) {
	valueOrdinal := field.ValueOrdinal()
	if uint64(valueOrdinal) >= uint64(len(state.folded)) {
		return heapdomain.CellState{}, false
	}
	if !state.folded[valueOrdinal] {
		return state.atomCell(field, state.bindings[valueOrdinal], keyContainment)
	}
	return state.foldedCell(field, keyContainment)
}

// atomCell is one alternative's contribution: raw absence for a nil
// alternative, otherwise one present tuple carrying its child edge.
func (state *application) atomCell(field source.Field, atom valuedomain.Atom, keyContainment heapdomain.Containment) (heapdomain.CellState, bool) {
	if atom.RuntimeKinds().Contains(runtimekind.Nil) {
		return state.schema.CellAbsent()
	}
	valueContainment, containmentOK := keymatch.Containment(state.schema, state.values, atom)
	if !containmentOK {
		return heapdomain.CellState{}, false
	}
	return state.schema.CellPresent(field.Slot(), field.Payload(), valueContainment, keyContainment)
}

// foldedCell is the value-level projection of one payload-only coordinate:
// one Present per distinct child edge its alternatives denote, plus raw
// absence when nil is among them.  Because the coordinate selects no key, the
// worlds its alternatives would fork differ in this cell alone, and their
// pointwise merge is exactly this union.
func (state *application) foldedCell(field source.Field, keyContainment heapdomain.Containment) (heapdomain.CellState, bool) {
	atoms := state.payloads[field.ValueOrdinal()]
	if len(atoms) == 0 {
		return heapdomain.CellState{}, false
	}
	merged, mergedOK := state.atomCell(field, atoms[0], keyContainment)
	if !mergedOK {
		return heapdomain.CellState{}, false
	}
	for _, atom := range atoms[1:] {
		cell, cellOK := state.atomCell(field, atom, keyContainment)
		if !cellOK {
			return heapdomain.CellState{}, false
		}
		union, unionOK := state.schema.CellUnion(merged, cell)
		if !unionOK {
			return heapdomain.CellState{}, false
		}
		merged = union
	}
	return merged, true
}
