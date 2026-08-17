// Package closed owns Heap's one atomic fixed-scalar table-constructor Rule.
// Link/source owns constructor topology and causal same-read witnesses; Value
// owns atoms; keymatch owns atom-to-key projection; Heap owns the complete
// object/world construction.  This package introduces no field Fact plane.
package closed

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func resultClosed(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, cells engine.OrderedCells[valuedomain.Value]) (next heapdomain.Value, normal, ok bool) {
	if values == nil || !operand.FencedTo(schema, values) || cells.Count() != operand.CoordinateCount() {
		return heapdomain.Value{}, false, false
	}
	inputs := make([]valuedomain.Value, cells.Count())
	for index := range inputs {
		value, present, available := cells.At(index)
		if !available {
			return heapdomain.Value{}, false, false
		}
		if !present {
			return heapdomain.Value{}, false, true
		}
		inputs[index] = value
	}
	return evaluateClosed(schema, values, operand, predecessor, inputs)
}

// evaluateClosed enumerates the sealed finite atom product without Go recursion.
// Every coordinate is picked once and reused at all of its source uses;
// source.Closed has already collapsed only Link-proved direct same-read pairs
// to one ordinal.  Distinct ordinals remain independent by construction.
func evaluateClosed(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, inputs []valuedomain.Value) (next heapdomain.Value, normal, ok bool) {
	if values == nil || len(inputs) != operand.CoordinateCount() || !schema.Admits(operand.Key(), predecessor) {
		return heapdomain.Value{}, false, false
	}
	fields, fieldsOK := fieldsFor(operand)
	choices, choicesOK := atomChoices(values, inputs)
	if !fieldsOK || !choicesOK {
		return heapdomain.Value{}, false, false
	}
	order, orderOK := coordinateOrder(fields, len(choices))
	none, noneOK := schema.ContainmentNone()
	base, baseOK := schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !orderOK || len(order) != len(choices) || !noneOK || !baseOK {
		return heapdomain.Value{}, false, false
	}

	bindings := make([]valuedomain.Atom, len(choices))
	bound := make([]bool, len(choices))
	var worlds []heapdomain.Value
	var occupied []bool

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
			bound[release] = false
			bindings[release] = valuedomain.Atom{}
		}
	}

	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if current.depth < 0 || current.depth >= len(order) {
			return heapdomain.Value{}, false, false
		}
		coordinate := order[current.depth]
		if coordinate < 0 || coordinate >= len(choices) || current.next >= len(choices[coordinate]) {
			pop()
			continue
		}
		atom := choices[coordinate][current.next]
		current.next++
		if bound[coordinate] {
			return heapdomain.Value{}, false, false
		}
		bindings[coordinate], bound[coordinate] = atom, true
		child := current.init
		nextField, branchNormal, applied := applyReady(schema, values, fields, bindings, bound, current.field, &child)
		if !applied {
			bound[coordinate], bindings[coordinate] = false, valuedomain.Atom{}
			return heapdomain.Value{}, false, false
		}
		if !branchNormal {
			bound[coordinate], bindings[coordinate] = false, valuedomain.Atom{}
			continue
		}
		if current.depth+1 != len(order) {
			stack = append(stack, frame{depth: current.depth + 1, field: nextField, init: child, release: coordinate})
			continue
		}
		if nextField != len(fields) {
			bound[coordinate], bindings[coordinate] = false, valuedomain.Atom{}
			return heapdomain.Value{}, false, false
		}
		fresh, freshOK := child.Finish()
		created, createdOK := schema.Create(predecessor, operand.Key(), fresh)
		bound[coordinate], bindings[coordinate] = false, valuedomain.Atom{}
		if !freshOK || !createdOK {
			return heapdomain.Value{}, false, false
		}
		if !accumulateWorld(&worlds, &occupied, created) {
			return heapdomain.Value{}, false, false
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

func finishWorlds(slots []heapdomain.Value, occupied []bool) (heapdomain.Value, bool, bool) {
	if len(slots) != len(occupied) {
		return heapdomain.Value{}, false, false
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
			return heapdomain.Value{}, false, false
		}
		result = joined
	}
	if !have {
		return heapdomain.Value{}, false, true
	}
	return result, true, true
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

// atomChoices intentionally uses Value.VisitSupport: Top includes every
// sealed atom, including opaque/reference alternatives that a kind-only scan
// would silently drop.  Its canonical iteration order drives deterministic
// enumeration; no Go map determines semantic output order.
func atomChoices(schema *valuedomain.Schema, inputs []valuedomain.Value) ([][]valuedomain.Atom, bool) {
	if schema == nil {
		return nil, false
	}
	choices := make([][]valuedomain.Atom, len(inputs))
	for index, input := range inputs {
		if input.IsBottom() || !schema.VisitSupport(input, func(atom valuedomain.Atom) {
			choices[index] = append(choices[index], atom)
		}) || len(choices[index]) == 0 {
			return nil, false
		}
	}
	return choices, true
}

// coordinateOrder is first source use (dynamic key then payload) with the
// descriptor's ordinal mapping retained for access to the canonical summary
// input.  It is an execution order, never a second coordinate vocabulary.
func coordinateOrder(fields []source.Field, count int) ([]int, bool) {
	if count <= 0 {
		return nil, false
	}
	seen := make([]bool, count)
	order := make([]int, 0, count)
	appendOrdinal := func(ordinal uint32) bool {
		if uint64(ordinal) >= uint64(count) {
			return false
		}
		index := int(ordinal)
		if !seen[index] {
			seen[index] = true
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
	return order, len(order) == count
}

// applyReady extends exactly the source-order prefix whose inputs are bound.
// The first unbound field is a hard frontier: later fields cannot move ahead
// of it, even when their coordinates happen to be selected already.
func applyReady(schema heapdomain.Schema, values *valuedomain.Schema, fields []source.Field, bindings []valuedomain.Atom, bound []bool, start int, init *heapdomain.ObjectInit) (next int, normal, ok bool) {
	if values == nil || init == nil || start < 0 || start > len(fields) || len(bindings) != len(bound) {
		return 0, false, false
	}
	for next = start; next < len(fields); next++ {
		field := fields[next]
		valueOrdinal := field.ValueOrdinal()
		if uint64(valueOrdinal) >= uint64(len(bindings)) || !bound[valueOrdinal] {
			return next, true, true
		}
		selector, keyContainment, selected, selectedOK := selectorFor(schema, values, field, bindings, bound)
		if !selectedOK {
			return 0, false, false
		}
		if !selected {
			return next, false, true
		}
		atom := bindings[valueOrdinal]
		var cell heapdomain.CellState
		var cellOK bool
		if atom.RuntimeKinds().Contains(runtimekind.Nil) {
			cell, cellOK = schema.CellAbsent()
		} else {
			valueContainment, containmentOK := keymatch.Containment(schema, values, atom)
			if !containmentOK {
				return 0, false, false
			}
			cell, cellOK = schema.CellPresent(field.Slot(), field.Payload(), valueContainment, keyContainment)
		}
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
func selectorFor(schema heapdomain.Schema, values *valuedomain.Schema, field source.Field, bindings []valuedomain.Atom, bound []bool) (heapdomain.KeySelector, heapdomain.Containment, bool, bool) {
	if selector, exact := field.ExactSelector(); exact {
		none, ok := schema.ContainmentNone()
		return selector, none, true, ok
	}
	ordinal, dynamic := field.DynamicKeyOrdinal()
	if !dynamic || uint64(ordinal) >= uint64(len(bindings)) || !bound[ordinal] {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false, false
	}
	atom := bindings[ordinal]
	if !atom.TableKeyValidity().MayBeValid() {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false, true
	}
	alternative, ok := keymatch.Project(schema, values, atom)
	if !ok {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false, false
	}
	return alternative.Selector(), alternative.Containment(), true, true
}
