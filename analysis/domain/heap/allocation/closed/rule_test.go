package closed

import (
	"context"
	"encoding/binary"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/ingress"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/analysis/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuesource "github.com/wippyai/go-lua/analysis/domain/value/source"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestClosedDirectReadDiagonalRetainsOnlySameAtomWorlds(t *testing.T) {
	heap, values, _, operand := closedFixture(t, `
local a = {}
local b = {}
local x = a
local table = { [x] = x }
return table
`, 1)
	field, fieldOK := operand.At(0)
	keyOrdinal, keyOK := field.DynamicKeyOrdinal()
	if !fieldOK || !keyOK || field.ValueOrdinal() != keyOrdinal || operand.CoordinateCount() != 1 {
		t.Fatal("direct read witness did not collapse key/payload to one coordinate")
	}
	left, right := twoEmptyTableAtoms(t, heap, values)
	input, inputOK := values.Alternatives(left, right)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	rule := semanticRule(t, heap, values)
	if !inputOK || !predecessorOK {
		t.Fatal("diagonal fixture")
	}
	got, normal, gotOK := rule.evaluate(operand, predecessor, []valuedomain.Value{input})
	leftWorld, leftOK := dynamicLeaf(heap, values, operand, predecessor, left, left)
	rightWorld, rightOK := dynamicLeaf(heap, values, operand, predecessor, right, right)
	want, joined := heapdomain.Join(leftWorld, rightWorld)
	if !gotOK || !normal || !leftOK || !rightOK || !joined || !heap.Domain().Equal(got, want) || got.WorldCount() != 2 {
		t.Fatal("direct same-read constructor admitted crossed key/value worlds")
	}
}

func TestClosedDistinctCoordinatesEnumerateFullIndependentProduct(t *testing.T) {
	heap, values, _, operand := closedFixture(t, `
local a = {}
local b = {}
local key = a
local value = b
local table = { [key] = value }
return table
`, 1)
	field, fieldOK := operand.At(0)
	keyOrdinal, keyOK := field.DynamicKeyOrdinal()
	if !fieldOK || !keyOK || field.ValueOrdinal() == keyOrdinal || operand.CoordinateCount() != 2 {
		t.Fatal("independent source coordinates were collapsed")
	}
	left, right := twoEmptyTableAtoms(t, heap, values)
	choice, choiceOK := values.Alternatives(left, right)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !choiceOK || !predecessorOK {
		t.Fatal("independent fixture")
	}
	inputs := []valuedomain.Value{choice, choice}
	rule := semanticRule(t, heap, values)
	got, normal, gotOK := rule.evaluate(operand, predecessor, inputs)
	var want heapdomain.Value
	first := true
	for _, keyAtom := range []valuedomain.Atom{left, right} {
		for _, valueAtom := range []valuedomain.Atom{left, right} {
			leaf, leafOK := dynamicLeaf(heap, values, operand, predecessor, keyAtom, valueAtom)
			if !leafOK {
				t.Fatal("independent expected leaf")
			}
			if first {
				want, first = leaf, false
			} else {
				var joinOK bool
				want, joinOK = heapdomain.Join(want, leaf)
				if !joinOK {
					t.Fatal("independent expected join")
				}
			}
		}
	}
	if !gotOK || !normal || !heap.Domain().Equal(got, want) || got.WorldCount() != 4 {
		t.Fatal("independent coordinates did not retain their complete product")
	}
}

// Binary-carry accumulation is a performance implementation of ordinary
// lattice Join, not a new world policy. Five non-dominated leaves exercise a
// partially occupied carry forest and catch dropped levels or final-fold
// ordering mistakes without inspecting private accumulator structure.
func TestClosedBalancedWorldAccumulatorMatchesOrdinaryJoin(t *testing.T) {
	heap, values, _, operand := closedFixture(t, `
local a = {}
local b = {}
local c = {}
local key = a
local value = b
return { [key] = value }
`, 1)
	atoms := emptyTableAtoms(t, heap, values, 3)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !predecessorOK {
		t.Fatal("accumulator predecessor")
	}
	pairs := [][2]valuedomain.Atom{
		{atoms[0], atoms[0]}, {atoms[1], atoms[1]}, {atoms[2], atoms[2]},
		{atoms[0], atoms[1]}, {atoms[1], atoms[2]},
	}
	var ordinary heapdomain.Value
	var slots []heapdomain.Value
	var occupied []bool
	for index, pair := range pairs {
		leaf, leafOK := dynamicLeaf(heap, values, operand, predecessor, pair[0], pair[1])
		if !leafOK || !accumulateWorld(&slots, &occupied, leaf) {
			t.Fatal("accumulator leaf")
		}
		if index == 0 {
			ordinary = leaf
			continue
		}
		var joinOK bool
		ordinary, joinOK = heapdomain.Join(ordinary, leaf)
		if !joinOK {
			t.Fatal("ordinary join")
		}
	}
	balanced, normal, balancedOK := finishWorlds(slots, occupied)
	if !balancedOK || !normal || !heap.Domain().Equal(balanced, ordinary) || balanced.WorldCount() != len(pairs) {
		t.Fatal("balanced accumulation changed complete Heap worlds")
	}
}

func TestClosedSourceOrderNilDeletionAndCreateRecurrence(t *testing.T) {
	heap, values, _, operand := closedFixture(t, `
local child = {}
local table = { item = child, item = nil }
return table
`, 2)
	child, childOK := firstEmptyTableAtom(heap, values)
	nilAtom, nilOK := sourceAtomFor(t, values, func(atom valuedomain.Atom) bool {
		return atom.RuntimeKinds() == runtimekind.Bit(runtimekind.Nil)
	})
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !childOK || !nilOK || !predecessorOK {
		t.Fatal("source-order fixture")
	}
	inputs := defaultInputs(t, values, operand)
	first, firstOK := operand.At(0)
	second, secondOK := operand.At(1)
	if !firstOK || !secondOK {
		t.Fatal("source-order fields")
	}
	inputs[first.ValueOrdinal()], _ = values.Singleton(child)
	inputs[second.ValueOrdinal()], _ = values.Singleton(nilAtom)
	rule := semanticRule(t, heap, values)
	got, normal, gotOK := rule.evaluate(operand, predecessor, inputs)
	want, wantOK := staticOverwriteLeaf(heap, values, operand, predecessor, child, nilAtom)
	if !gotOK || !normal || !wantOK || !heap.Domain().Equal(got, want) {
		t.Fatal("later nil did not delete the earlier source-order field")
	}
	again, againNormal, againOK := rule.evaluate(operand, got, inputs)
	world, worldOK := again.WorldAt(0)
	if !againOK || !againNormal || !worldOK || world.Kind() != heapdomain.WorldMany {
		t.Fatal("closed constructor did not use Create recurrence")
	}
}

func TestClosedInvalidKeyContributesNoNormalHeapSuccessor(t *testing.T) {
	heap, values, _, operand := closedFixture(t, `local n = nil; local x = {}; return { [x] = x }`, 1)
	nilAtom, nilOK := sourceAtomFor(t, values, func(atom valuedomain.Atom) bool {
		return atom.RuntimeKinds() == runtimekind.Bit(runtimekind.Nil)
	})
	nilValue, valueOK := values.Singleton(nilAtom)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !nilOK || !valueOK || !predecessorOK {
		t.Fatal("invalid-key fixture")
	}
	got, normal, resultOK := semanticRule(t, heap, values).evaluate(operand, predecessor, []valuedomain.Value{nilValue})
	if !resultOK || normal || got.Valid() {
		t.Fatal("invalid dynamic key fabricated a normal Heap successor")
	}
}

func TestClosedCarriesOpaqueContainmentForKeyAndValue(t *testing.T) {
	heap, values, _, operand := closedFixture(t, `local x = {}; return { [x] = x }`, 1)
	opaque, opaqueOK := values.OpaqueReference(valuedomain.ReferenceTable)
	input, inputOK := values.Singleton(opaque)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !opaqueOK || !inputOK || !predecessorOK {
		t.Fatal("opaque containment fixture")
	}
	rule := semanticRule(t, heap, values)
	got, normal, gotOK := rule.evaluate(operand, predecessor, []valuedomain.Value{input})
	want, wantOK := dynamicLeaf(heap, values, operand, predecessor, opaque, opaque)
	if !gotOK || !normal || !wantOK || !heap.Domain().Equal(got, want) {
		t.Fatal("opaque key/value containment was lost or treated as none")
	}
}

func TestClosedOwnerFencingAndForgedEvidenceFailClosed(t *testing.T) {
	heap, values, linked, operand := closedFixture(t, `local x = {}; return { [x] = x }`, 1)
	rule := semanticRule(t, heap, values)
	otherHeap, otherHeapOK := heapdomain.Seal(linked)
	otherValues, otherValuesOK := valuedomain.Seal(linked, otherHeap)
	foreign, foreignOK := newOperand(otherHeap, otherValues, allocationRootFor(t, otherHeap, 1))
	if !otherHeapOK || !otherValuesOK || !foreignOK || !operand.FencedTo(heap, values) || operand.FencedTo(otherHeap, values) || operand.FencedTo(heap, otherValues) || operand.RevalidateFor(otherHeap, values) || operand.RevalidateFor(heap, otherValues) {
		t.Fatal("closed source schema fence")
	}
	foreignPredecessor, predecessorOK := otherHeap.EmptyObject(foreign.Key())
	foreignAtom, atomOK := otherValues.OpaqueKind(runtimekind.String)
	foreignInput, inputOK := otherValues.Singleton(foreignAtom)
	if !predecessorOK || !atomOK || !inputOK {
		t.Fatal("foreign evaluation fixture")
	}
	if _, _, accepted := rule.evaluate(foreign, foreignPredecessor, []valuedomain.Value{foreignInput}); accepted {
		t.Fatal("foreign closed descriptor entered local Rule")
	}

	composition := engine.NewComposition()
	heapOwner, heapOK := heapowner.Declare(composition, closedKey(50), heap)
	valueOwner, valueOK := valueowner.Declare(composition, closedKey(51), closedKey(52), values)
	foreignComposition := engine.NewComposition()
	foreignHeapOwner, foreignHeapOK := heapowner.Declare(foreignComposition, closedKey(57), otherHeap)
	foreignValueOwner, foreignValueOK := valueowner.Declare(foreignComposition, closedKey(58), closedKey(59), otherValues)
	if !foreignHeapOK || !foreignValueOK {
		t.Fatal("same-content foreign owners")
	}
	if _, accepted := Declare(foreignComposition, closedKey(60), closedKey(61), closedKey(62), closedKey(63), heapOwner, foreignValueOwner); accepted {
		t.Fatal("same-content foreign Value owner crossed closed declaration fence")
	}
	if _, accepted := Declare(foreignComposition, closedKey(64), closedKey(65), closedKey(66), closedKey(67), foreignHeapOwner, valueOwner); accepted {
		t.Fatal("same-content foreign Heap owner crossed closed declaration fence")
	}
	localComposition := engine.NewComposition()
	localHeapOwner, localHeapOK := heapowner.Declare(localComposition, closedKey(68), otherHeap)
	localValueOwner, localValueOK := valueowner.Declare(localComposition, closedKey(69), closedKey(70), otherValues)
	localRule, localRuleOK := Declare(localComposition, closedKey(71), closedKey(72), closedKey(73), closedKey(74), localHeapOwner, localValueOwner)
	if !localHeapOK || !localValueOK || !localRuleOK || localRule == nil {
		t.Fatal("same-content local owner pair was rejected")
	}
	declared, ruleOK := Declare(composition, closedKey(53), closedKey(54), closedKey(56), closedKey(55), heapOwner, valueOwner)
	if !heapOK || !valueOK || !ruleOK || declared == nil {
		t.Fatal("closed declaration")
	}
	if evidence, accepted := declared.check(engine.RuleDerivation[heapdomain.Value, source.Closed]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged closed derivation minted evidence")
	}
}

// This is the production path: Heap ingress and Value source are two ordinary
// zero-input writers at one source point. The closed Rule consumes their one
// guarded predecessor through its actual engine transfer and derivation
// checker. A visible complete table world therefore proves admission, not a
// direct call to evaluate.
func TestClosedRuleAssembledProductionExecutionAcceptsDerivation(t *testing.T) {
	heap, values, _, operand := closedFixture(t, `return { answer = 1 }`, 1)
	field, fieldOK := operand.At(0)
	coordinate, coordinateOK := operand.CoordinateAt(int(field.ValueOrdinal()))
	seed, seedOK := integerSeed(t, values, coordinate)
	root := allocationRootFor(t, heap, 1)
	if !fieldOK || !coordinateOK || !seedOK {
		t.Fatal("assembled fixture source")
	}
	runClosedProduction(t, heap, values, root, operand, seed)
}

func runClosedProduction(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema, root heapdomain.Key, operand source.Closed, seed valuedomain.SourceSeed) {
	t.Helper()
	composition := engine.NewComposition()
	heapOwner, heapOK := heapowner.Declare(composition, closedKey(100), heap)
	valueOwner, valueOK := valueowner.Declare(composition, closedKey(101), closedKey(102), values)
	ingressRule, ingressOK := ingress.Declare(composition, closedKey(103), closedKey(104), closedKey(105), heapOwner)
	valueRule, valueRuleOK := valuesource.Declare(composition, closedKey(106), closedKey(107), closedKey(108), valueOwner)
	closedRule, closedOK := Declare(composition, closedKey(109), closedKey(110), closedKey(150), closedKey(111), heapOwner, valueOwner)
	if !heapOK || !valueOK || !ingressOK || !valueRuleOK || !closedOK {
		t.Fatal("assembled declarations")
	}
	var read engine.QueryRead[engine.OrderedCells[heapdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: closedKey(112),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				if !cellsOK || cells.Count() != 1 {
					return false
				}
				value, present, cellOK := cells.At(0)
				world, worldOK := value.WorldAt(0)
				return rows == 1 && cellOK && present && worldOK && world.Kind() == heapdomain.WorldOne
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: closedKey(113), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value }, Equal: func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		read, declared = engine.QueryReadFrom(query, heapOwner.ExactRead())
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("assembled query/seal")
	}
	ingressInstance, ingressInstanceOK := ingressRule.Instance(root)
	valueInstance, valueInstanceOK := valueRule.Instance(seed)
	closedInstance, closedInstanceOK := closedRule.Instance(root)
	rootRef, rootRefOK := heapOwner.Locate(operand.Key())
	if !ingressInstanceOK || !valueInstanceOK || !closedInstanceOK || !rootRefOK {
		t.Fatal("assembled instances")
	}
	result := testlaw.RunTwo(context.Background(), testlaw.TwoFixture[
		heapdomain.Value, source.Root,
		valuedomain.Value, valuedomain.SourceSeed,
		heapdomain.Value, source.Closed,
		bool,
	]{
		Composition: composition, First: ingressInstance, Second: valueInstance, Target: closedInstance, Query: query,
		BindQuery:  func(binding *engine.QueryBinding[bool]) bool { return engine.InstanceQueryRead(binding, read, rootRef) },
		SourceSite: closedKey(114), FirstOccurrence: closedKey(115), SecondOccurrence: closedKey(116), TargetSite: closedKey(117), TargetOccurrence: closedKey(118), BoundarySemantic: closedKey(119),
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("closed production execution = status:%v available:%t result:%t", result.Status, result.ValueAvailable, result.Value)
	}
}

func semanticRule(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema) *Rule {
	t.Helper()
	composition := engine.NewComposition()
	heapOwner, heapOK := heapowner.Declare(composition, closedKey(1), heap)
	valueOwner, valueOK := valueowner.Declare(composition, closedKey(2), closedKey(3), values)
	if !heapOK || !valueOK {
		t.Fatal("semantic rule owners")
	}
	return &Rule{heap: heapOwner, values: valueOwner}
}

func closedFixture(t testing.TB, text string, fieldCount int) (heapdomain.Schema, *valuedomain.Schema, *link.Link, source.Closed) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "closed_rule.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heap, heapOK := heapdomain.Seal(linked)
	values, valuesOK := valuedomain.Seal(linked, heap)
	if !heapOK || !valuesOK {
		t.Fatal("closed schemas")
	}
	root := allocationRootFor(t, heap, fieldCount)
	operand, operandOK := newOperand(heap, values, root)
	if !operandOK {
		t.Fatal("closed operand")
	}
	return heap, values, linked, operand
}

func allocationRootFor(t testing.TB, schema heapdomain.Schema, fields int) heapdomain.Key {
	t.Helper()
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		_, _, kind, originOK := root.ProgramAllocation()
		if rootOK && originOK && kind == heapdomain.AllocationTable && schema.FieldCount(root) == fields {
			return root
		}
	}
	t.Fatalf("allocation root with %d fields", fields)
	return heapdomain.Key{}
}

func twoEmptyTableAtoms(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema) (valuedomain.Atom, valuedomain.Atom) {
	t.Helper()
	atoms := emptyTableAtoms(t, heap, values, 2)
	return atoms[0], atoms[1]
}

func emptyTableAtoms(t testing.TB, schema heapdomain.Schema, values *valuedomain.Schema, count int) []valuedomain.Atom {
	t.Helper()
	if count <= 0 {
		t.Fatal("empty table atom count")
	}
	var atoms []valuedomain.Atom
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		_, _, kind, originOK := root.ProgramAllocation()
		if !rootOK || !originOK || kind != heapdomain.AllocationTable || schema.FieldCount(root) != 0 {
			continue
		}
		atom, atomOK := values.Allocation(root, materialization.Recent)
		if !atomOK {
			t.Fatal("allocation atom")
		}
		atoms = append(atoms, atom)
	}
	if len(atoms) < count {
		t.Fatalf("%d empty allocation atoms", count)
	}
	return atoms[:count]
}

func firstEmptyTableAtom(schema heapdomain.Schema, values *valuedomain.Schema) (valuedomain.Atom, bool) {
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		_, _, kind, originOK := root.ProgramAllocation()
		if rootOK && originOK && kind == heapdomain.AllocationTable && schema.FieldCount(root) == 0 {
			return values.Allocation(root, materialization.Recent)
		}
	}
	return valuedomain.Atom{}, false
}

func sourceAtomFor(t testing.TB, values *valuedomain.Schema, match func(valuedomain.Atom) bool) (valuedomain.Atom, bool) {
	t.Helper()
	for index := 0; index < values.Link().Boundary().Values().Count(); index++ {
		value, valueOK := values.Link().Boundary().Values().At(index)
		atom, atomOK := values.Source(value)
		if valueOK && atomOK && match(atom) {
			return atom, true
		}
	}
	return valuedomain.Atom{}, false
}

func defaultInputs(t testing.TB, values *valuedomain.Schema, operand source.Closed) []valuedomain.Value {
	t.Helper()
	atom, atomOK := values.OpaqueKind(runtimekind.String)
	value, valueOK := values.Singleton(atom)
	if !atomOK || !valueOK {
		t.Fatal("default input")
	}
	inputs := make([]valuedomain.Value, operand.CoordinateCount())
	for index := range inputs {
		inputs[index] = value
	}
	return inputs
}

func dynamicLeaf(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, keyAtom, valueAtom valuedomain.Atom) (heapdomain.Value, bool) {
	field, fieldOK := operand.At(0)
	alternative, alternativeOK := keymatch.Project(schema, values, keyAtom)
	none, noneOK := schema.ContainmentNone()
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !fieldOK || !alternativeOK || !noneOK || !initializerOK {
		return heapdomain.Value{}, false
	}
	valueContainment, containmentOK := keymatch.Containment(schema, values, valueAtom)
	cell, cellOK := schema.CellPresent(field.Slot(), field.Payload(), valueContainment, alternative.Containment())
	if !containmentOK || !cellOK || !initializer.Apply(alternative.Selector(), cell) {
		return heapdomain.Value{}, false
	}
	fresh, freshOK := initializer.Finish()
	if !freshOK {
		return heapdomain.Value{}, false
	}
	return schema.Create(predecessor, operand.Key(), fresh)
}

func staticOverwriteLeaf(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, child, nilAtom valuedomain.Atom) (heapdomain.Value, bool) {
	none, noneOK := schema.ContainmentNone()
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !noneOK || !initializerOK {
		return heapdomain.Value{}, false
	}
	for index := 0; index < operand.Count(); index++ {
		field, fieldOK := operand.At(index)
		selector, selectorOK := field.ExactSelector()
		if !fieldOK || !selectorOK {
			return heapdomain.Value{}, false
		}
		if index == 0 {
			valueContainment, containmentOK := keymatch.Containment(schema, values, child)
			cell, cellOK := schema.CellPresent(field.Slot(), field.Payload(), valueContainment, none)
			if !containmentOK || !cellOK || !initializer.Apply(selector, cell) {
				return heapdomain.Value{}, false
			}
			continue
		}
		if !nilAtom.RuntimeKinds().Contains(runtimekind.Nil) {
			return heapdomain.Value{}, false
		}
		cell, cellOK := schema.CellAbsent()
		if !cellOK || !initializer.Apply(selector, cell) {
			return heapdomain.Value{}, false
		}
	}
	fresh, freshOK := initializer.Finish()
	if !freshOK {
		return heapdomain.Value{}, false
	}
	return schema.Create(predecessor, operand.Key(), fresh)
}

func integerSeed(t testing.TB, values *valuedomain.Schema, want valuedomain.Coordinate) (valuedomain.SourceSeed, bool) {
	t.Helper()
	for index := 0; index < values.Link().Boundary().Values().Count(); index++ {
		seed, seedOK := values.SourceSeedAt(index)
		coordinate, fact, resultOK := seed.Result()
		if seedOK && resultOK && values.RuntimeKinds(fact) == runtimekind.Bit(runtimekind.Number) && coordinate == want {
			return seed, true
		}
	}
	return valuedomain.SourceSeed{}, false
}

func closedKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("closed key")
	}
	return key
}
