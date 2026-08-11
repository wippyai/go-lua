// Package closed owns Heap's one atomic fixed-scalar table-constructor Rule.
// Link/source owns constructor topology and causal same-read witnesses; Value
// owns atoms; keymatch owns atom-to-key projection; Heap owns the complete
// object/world construction.  This package introduces no field Fact plane.
package closed

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/analysis/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// newOperand derives Source's sole closed scalar-table operand. The descriptor
// remains private to this owner: callers bind an existing Heap allocation Key
// through Instance and cannot carry an independently reconstructed field
// vector into the Rule.
func newOperand(heap heapdomain.Schema, values *valuedomain.Schema, root heapdomain.Key) (source.Closed, bool) {
	return source.NewClosed(heap, values, root)
}

// Rule reads the predecessor Heap image and the operand's one canonical Value
// summary vector at the same input/guard, ages the carried Heap image for its
// root, and writes that exact allocation root once in the same patch.
type Rule struct {
	rule      *engine.Rule[heapdomain.Value, source.Closed]
	heapRead  engine.Read[engine.OrderedCells[heapdomain.Value]]
	valueRead engine.Read[engine.OrderedCells[valuedomain.Value]]
	write     engine.Write[heapdomain.Value]
	heap      *heapowner.Owner
	values    *valueowner.Owner
	semantic  engine.SemanticKey
	transform engine.SemanticKey
}

func Declare(composition *engine.Composition, semantic, family, transform, evidence engine.SemanticKey, heap *heapowner.Owner, values *valueowner.Owner) (*Rule, bool) {
	heapSchema := heapSchemaOf(heap)
	valueSchema := valueSchemaOf(values)
	if composition == nil || heapSchema == nil || valueSchema == nil || valueSchema.Link() == nil ||
		heapSchema.Link() == nil || valueSchema.Link() != heapSchema.Link() || !valueSchema.OwnsHeapSchema(*heapSchema) ||
		!distinct(semantic, family, transform, evidence) {
		return nil, false
	}
	declaration := &Rule{heap: heap, values: values, semantic: semantic, transform: transform}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[heapdomain.Value, source.Closed]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.content, Output: heap.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[heapdomain.Value, source.Closed]) bool {
		input, inputOK := rule.InputAt(0)
		if !inputOK {
			return false
		}
		heapRead, heapReadOK := engine.ReadFrom(rule, input, heap.ExactRead())
		valueRead, valueReadOK := engine.ReadFrom(rule, input, values.SummaryRead())
		write, writeOK := engine.WriteTo(rule, heap.ExactWrite())
		if !heapReadOK || !valueReadOK || !writeOK || !engine.TransformCarryFrom(rule, input, heap.Carry(), transform, func(operand source.Closed, prior heapdomain.Value) (heapdomain.Value, bool) {
			return heap.Schema().Age(prior, operand.Key())
		}) {
			return false
		}
		declaration.rule, declaration.heapRead, declaration.valueRead, declaration.write = rule, heapRead, valueRead, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

// heapSchemaOf/valueSchemaOf keep the declaration fence readable and ensure
// every owner capability is checked before any engine Rule is declared.  A
// matching Link ContentID is only a replay identity; it cannot authorize a
// live Value/Heap owner pair.
func heapSchemaOf(owner *heapowner.Owner) *heapdomain.Schema {
	if owner == nil {
		return nil
	}
	schema := owner.Schema()
	if !schema.Valid() {
		return nil
	}
	return &schema
}

func valueSchemaOf(owner *valueowner.Owner) *valuedomain.Schema {
	if owner == nil {
		return nil
	}
	schema := owner.Schema()
	if schema == nil || schema.Link() == nil {
		return nil
	}
	return schema
}

// Instance binds one existing Heap allocation Key. It is the sole public
// allocation-constructor entry point; source.Closed remains an owner-private
// Link projection used only by transfer, evidence, and cold binding.
func (rule *Rule) Instance(root heapdomain.Key) (*engine.RuleInstance[heapdomain.Value, source.Closed], bool) {
	if rule == nil || rule.heap == nil || rule.values == nil {
		return nil, false
	}
	operand, ok := newOperand(rule.heap.Schema(), rule.values.Schema(), root)
	if !ok {
		return nil, false
	}
	return rule.instance(operand)
}

func (rule *Rule) instance(operand source.Closed) (*engine.RuleInstance[heapdomain.Value, source.Closed], bool) {
	if rule == nil || rule.rule == nil || !rule.admitOperand(operand) {
		return nil, false
	}
	ref, refOK := rule.heap.Locate(operand.Key())
	refs := rule.values.NewSummaryRefs()
	if !refOK || refs == nil {
		return nil, false
	}
	for index := 0; index < operand.CoordinateCount(); index++ {
		coordinate, coordinateOK := operand.CoordinateAt(index)
		if !coordinateOK || !rule.values.AppendSummaryCoordinate(refs, coordinate) {
			return nil, false
		}
	}
	if !rule.values.CloseSummaryRefs(refs) {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[heapdomain.Value, source.Closed]) bool {
		return engine.InstanceRead(binding, rule.heapRead, ref) &&
			engine.InstanceSummaryRead(binding, rule.valueRead, rule.values.SummaryRead(), refs) &&
			engine.InstanceWrite(binding, rule.write, ref)
	})
}

func (rule *Rule) validOperand(operand source.Closed) bool {
	return rule != nil && rule.heap != nil && rule.values != nil && rule.values.Schema() != nil &&
		operand.FencedTo(rule.heap.Schema(), rule.values.Schema())
}

// admitOperand is deliberately cold: it reconstructs Link-owned field
// topology once at rule-content, binding, and evidence boundaries. Hot
// transfer paths use validOperand's owner/capability fence only; no product
// row may re-sort or re-derive this sealed descriptor.
func (rule *Rule) admitOperand(operand source.Closed) bool {
	return rule.validOperand(operand) && operand.RevalidateFor(rule.heap.Schema(), rule.values.Schema())
}

func (rule *Rule) content(operand source.Closed) (source.Closed, [32]byte, bool) {
	if !rule.admitOperand(operand) {
		return source.Closed{}, [32]byte{}, false
	}
	id, ok := operand.ID()
	return operand, [32]byte(id), ok
}

func (rule *Rule) transfer(access engine.Access[heapdomain.Value, source.Closed]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		predecessorCells, predecessorRead := engine.ReadValue(access, row, rule.heapRead)
		inputs, inputsRead := engine.ReadValue(access, row, rule.valueRead)
		if !predecessorRead || !inputsRead || predecessorCells.Count() != 1 {
			return false
		}
		predecessor, predecessorPresent, predecessorAvailable := predecessorCells.At(0)
		if !predecessorAvailable {
			return false
		}
		if !predecessorPresent {
			return engine.NoCandidate(access, row)
		}
		next, normal, resultOK := rule.result(operand, predecessor, inputs)
		if !resultOK {
			return false
		}
		if !normal {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next)
	})
}

// result is the only closed-constructor reduction.  It is called by transfer
// and its admission checker, so the evidence path cannot grow a second Heap
// evaluator.  normal=false denotes no valid *normal* constructor successor;
// invalid-key diagnostics are handled by their own outcome Rule.
func (rule *Rule) result(operand source.Closed, predecessor heapdomain.Value, cells engine.OrderedCells[valuedomain.Value]) (next heapdomain.Value, normal, ok bool) {
	if !rule.validOperand(operand) || cells.Count() != operand.CoordinateCount() {
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
	return rule.evaluate(operand, predecessor, inputs)
}

// evaluate enumerates the sealed finite atom product without Go recursion.
// Every coordinate is picked once and reused at all of its source uses;
// source.Closed has already collapsed only Link-proved direct same-read pairs
// to one ordinal.  Distinct ordinals remain independent by construction.
func (rule *Rule) evaluate(operand source.Closed, predecessor heapdomain.Value, inputs []valuedomain.Value) (next heapdomain.Value, normal, ok bool) {
	if !rule.validOperand(operand) || len(inputs) != operand.CoordinateCount() {
		return heapdomain.Value{}, false, false
	}
	schema, values := rule.heap.Schema(), rule.values.Schema()
	if values == nil || !schema.Admits(operand.Key(), predecessor) {
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

func (rule *Rule) check(derivation engine.RuleDerivation[heapdomain.Value, source.Closed]) (engine.RuleEvidence, bool) {
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 2 || derivation.DispositionCount() == 0 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	id, idOK := operand.ID()
	ref, refOK := rule.heap.Locate(operand.Key())
	refs := rule.values.NewSummaryRefs()
	if !operandOK || !idOK || !rule.admitOperand(operand) || !refOK || refs == nil || !derivation.OperandContentMatches([32]byte(id)) {
		return engine.RuleEvidence{}, false
	}
	for index := 0; index < operand.CoordinateCount(); index++ {
		coordinate, coordinateOK := operand.CoordinateAt(index)
		if !coordinateOK || !rule.values.AppendSummaryCoordinate(refs, coordinate) {
			return engine.RuleEvidence{}, false
		}
	}
	if !rule.values.CloseSummaryRefs(refs) || !engine.DerivationReadMatchesRef(derivation, rule.heapRead, ref) ||
		!engine.DerivationReadMatchesSummaryRefs(derivation, rule.valueRead, refs) {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	if !inputOK || input.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	for index := 0; index < derivation.DispositionCount(); index++ {
		disposition, dispositionOK := derivation.DispositionAt(index)
		if !dispositionOK || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		predecessorCells, predecessorOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.heapRead)
		inputs, inputsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.valueRead)
		if !predecessorOK || !inputsOK || predecessorCells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		predecessor, predecessorPresent, predecessorAvailable := predecessorCells.At(0)
		if !predecessorAvailable {
			return engine.RuleEvidence{}, false
		}
		if !predecessorPresent {
			_, transformed := disposition.CarryTransform()
			if disposition.Kind() != engine.RuleDispositionNoCandidate || transformed || disposition.TransformOnly() || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		want, normal, resultOK := rule.result(operand, predecessor, inputs)
		if !resultOK {
			return engine.RuleEvidence{}, false
		}
		if !normal {
			_, transformed := disposition.CarryTransform()
			if disposition.Kind() != engine.RuleDispositionNoCandidate || transformed || disposition.TransformOnly() || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		actual, actualOK := disposition.Value()
		target, targetOK := disposition.TargetAt(0)
		transform, transformed := disposition.CarryTransform()
		if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !actualOK || !targetOK ||
			disposition.TransformOnly() || !transformed || transform != rule.transform ||
			!engine.TargetMatchesRef(target, ref) || !rule.heap.Schema().Domain().Equal(actual, want) {
			return engine.RuleEvidence{}, false
		}
	}
	return derivation.Accept()
}

func distinct(keys ...engine.SemanticKey) bool {
	for index, key := range keys {
		if !key.Available() {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if keys[prior] == key {
				return false
			}
		}
	}
	return true
}
