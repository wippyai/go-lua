package closed_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestClosedSemanticDiagonalKeepsSameCoordinateAndOpaqueContainment(t *testing.T) {
	heap, values, operand := closedSemanticFixture(t, `local a = {}; local b = {}; local x = a; return { [x] = x }`)
	if operand.CoordinateCount() != 1 {
		t.Fatalf("same-read coordinates=%d, want one", operand.CoordinateCount())
	}
	field, ok := operand.At(0)
	dynamicOrdinal, dynamicOK := field.DynamicKeyOrdinal()
	if !ok || !dynamicOK || field.ValueOrdinal() != dynamicOrdinal {
		t.Fatal("closed field did not retain the direct same-read diagonal")
	}
	left, right := tableAtoms(t, heap, values, 2)
	_, alternativesOK := values.Alternatives(left, right)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !alternativesOK || !predecessorOK {
		t.Fatal("diagonal fixture")
	}
	leftWorld, leftOK := materializeField(heap, operand, left)
	rightWorld, rightOK := materializeField(heap, operand, right)
	joined, joinedOK := heapdomain.Join(leftWorld, rightWorld)
	if !leftOK || !rightOK || !joinedOK || joined.WorldCount() != 2 {
		t.Fatalf("same-read diagonal materialization left=%t right=%t join=%t worlds=%d", leftOK, rightOK, joinedOK, joined.WorldCount())
	}

	opaque, opaqueOK := values.OpaqueReference(valuedomain.ReferenceTable)
	_, inputOK := values.Singleton(opaque)
	_, unknownOK := heap.ContainmentUnknown()
	if !opaqueOK || !inputOK || !predecessor.Valid() || !unknownOK {
		t.Fatal("opaque key/value did not survive closed containment")
	}
}

func TestClosedSemanticDistinctCoordinatesEnumerateIndependentProduct(t *testing.T) {
	heap, values, operand := closedSemanticFixture(t, `local a = {}; local b = {}; local key = a; local value = b; return { [key] = value }`)
	if operand.CoordinateCount() != 2 {
		t.Fatalf("independent coordinates=%d, want two", operand.CoordinateCount())
	}
	left, right := tableAtoms(t, heap, values, 2)
	_, alternativesOK := values.Alternatives(left, right)
	_, predecessorOK := heap.EmptyObject(operand.Key())
	if !alternativesOK || !predecessorOK {
		t.Fatal("independent fixture")
	}
	leftWorld, leftOK := materializeField(heap, operand, left)
	rightWorld, rightOK := materializeField(heap, operand, right)
	if !leftOK || !rightOK {
		t.Fatal("independent field materialization")
	}
	joined, joinedOK := heapdomain.Join(leftWorld, rightWorld)
	if !joinedOK || joined.WorldCount() != 2 {
		t.Fatalf("independent product join=%t worlds=%d", joinedOK, joined.WorldCount())
	}
}

func TestClosedSemanticSourceOrderNilDeletionAndCreateRecurrence(t *testing.T) {
	heap, values, operand := closedSemanticFixture(t, `local child = {}; return { item = child, item = nil }`)
	if operand.Count() != 2 || operand.CoordinateCount() != 2 {
		t.Fatalf("source-order fields=%d coordinates=%d, want 2/2", operand.Count(), operand.CoordinateCount())
	}
	child, childOK := firstTableAtom(t, heap, values)
	nilAtom, nilOK := values.OpaqueKind(runtimekind.Nil)
	childValue, childValueOK := values.Singleton(child)
	nilValue, nilValueOK := values.Singleton(nilAtom)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !childOK || !nilOK || !childValueOK || !nilValueOK || !predecessorOK {
		t.Fatal("source-order fixture atoms")
	}
	inputs := []valuedomain.Value{values.Top(), values.Top()}
	for index := 0; index < operand.Count(); index++ {
		field, fieldOK := operand.At(index)
		if !fieldOK {
			t.Fatal("source-order field")
		}
		if index == 0 {
			inputs[field.ValueOrdinal()] = childValue
		} else {
			inputs[field.ValueOrdinal()] = nilValue
		}
	}
	if !predecessor.Valid() || inputs[0].IsBottom() || inputs[1].IsBottom() {
		t.Fatal("source-order constructor inputs")
	}
	fresh, freshOK := emptyMutableObject(heap)
	first, firstOK := heap.Create(predecessor, operand.Key(), fresh)
	second, secondOK := heap.Create(first, operand.Key(), fresh)
	world, worldOK := second.WorldAt(0)
	if !freshOK || !firstOK || !secondOK || !worldOK || world.Kind() != heapdomain.WorldMany {
		t.Fatal("Create recurrence did not age the previous recent world")
	}
}

func TestClosedSemanticInvalidDynamicKeyHasNoNormalSuccessor(t *testing.T) {
	heap, values, operand := closedSemanticFixture(t, `local x = {}; return { [x] = x }`)
	nilAtom, nilOK := values.OpaqueKind(runtimekind.Nil)
	_, valueOK := values.Singleton(nilAtom)
	_, predecessorOK := heap.EmptyObject(operand.Key())
	if !nilOK || !valueOK || !predecessorOK || operand.CoordinateCount() == 0 {
		t.Fatal("invalid dynamic key was not represented by a source coordinate")
	}
}

func materializeField(schema heapdomain.Schema, operand source.Closed, atom valuedomain.Atom) (heapdomain.Value, bool) {
	field, fieldOK := operand.At(0)
	none, noneOK := schema.ContainmentNone()
	cell, cellOK := schema.CellPresent(field.Slot(), field.Payload(), none, none)
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	selector, selectorOK := schema.KindSelector()
	if !fieldOK || !noneOK || !cellOK || !initializerOK || !selectorOK || !atom.RuntimeKinds().Contains(runtimekind.Table) || !initializer.Apply(selector, cell) {
		return heapdomain.Value{}, false
	}
	object, objectOK := initializer.Finish()
	world, worldOK := schema.One(operand.Key(), object)
	if !objectOK || !worldOK {
		return heapdomain.Value{}, false
	}
	return schema.Relation(operand.Key(), world)
}

func emptyMutableObject(schema heapdomain.Schema) (heapdomain.Object, bool) {
	none, noneOK := schema.ContainmentNone()
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !noneOK || !initializerOK {
		return heapdomain.Object{}, false
	}
	return initializer.Finish()
}

func closedSemanticFixture(t testing.TB, text string) (heapdomain.Schema, *valuedomain.Schema, source.Closed) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "closed_semantic.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := programschema.Global()
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	artifact, failure := schemaadapter.CompileDetailed(program.TransformerInput(), receipt)
	mount, mountOK := heapdomain.NewArtifactMount(artifact, module, programID)
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{mount})
	if !receiptOK || !shardOK || !moduleOK || !programIDOK || failure.Available() || !mountOK || heapFailure != heapdomain.SealFailureNone {
		t.Fatal("closed semantic artifact admission")
	}
	// Value's canonical mount is issued from the same artifact receipt; keep
	// this conversion local so the law never uses the retired Link fixture.
	valueMount, valueMountOK := valuedomain.NewArtifactMount(artifact, module, programID)
	if !valueMountOK {
		t.Fatal("closed semantic Value mount")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, []valuedomain.ArtifactMount{valueMount})
	if valueFailure != valuedomain.SealFailureNone {
		t.Fatal("closed semantic Value seal")
	}
	for index := 0; index < heap.KeyCount(); index++ {
		key, keyOK := heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		operand, operandOK := source.NewClosed(heap, values, key)
		if operandOK {
			return heap, values, operand
		}
	}
	t.Fatal("closed semantic source")
	return heapdomain.Schema{}, nil, source.Closed{}
}

func tableAtoms(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema, want int) (valuedomain.Atom, valuedomain.Atom) {
	t.Helper()
	atoms := make([]valuedomain.Atom, 0, want)
	for index := 0; index < heap.KeyCount() && len(atoms) < want; index++ {
		key, keyOK := heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		atom, atomOK := values.Allocation(key, materialization.Recent)
		if atomOK {
			atoms = append(atoms, atom)
		}
	}
	if len(atoms) < want {
		t.Fatalf("table atoms=%d, want %d", len(atoms), want)
	}
	return atoms[0], atoms[1]
}

func firstTableAtom(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema) (valuedomain.Atom, bool) {
	t.Helper()
	for index := 0; index < heap.KeyCount(); index++ {
		key, keyOK := heap.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		if atom, atomOK := values.Allocation(key, materialization.Recent); atomOK {
			return atom, true
		}
	}
	return valuedomain.Atom{}, false
}
