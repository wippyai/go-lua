package heap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	proglink "github.com/wippyai/go-lua/program/link"
)

// TestHeapAgeFastPathAndFences keeps the hot no-op image and the public
// ownership/allocation-key boundary explicit.  In particular, an unrelated
// finite Heap value must retain its immutable backing representation.
func TestHeapAgeFastPathAndFences(t *testing.T) {
	linked, schema := heapFixture(t, "heap_age_fast_path")
	selected, other := heapAgeAllocationKeys(t, linked, schema)
	otherRecent, otherRecentOK := schema.Reference(other, materialization.Recent)
	otherContainment, otherContainmentOK := schema.ContainmentExact(otherRecent)
	object, objectOK := schema.Object(ShapeEligible, FrozenMutable, otherContainment)
	world, worldOK := schema.One(selected, object)
	unchanged, unchangedOK := schema.Relation(selected, world)
	if !otherRecentOK || !otherContainmentOK || !objectOK || !worldOK || !unchangedOK {
		t.Fatal("unrelated-root Age fixture")
	}

	for _, value := range []Value{schema.Bottom(), schema.Top(), unchanged} {
		aged, ok := schema.Age(value, selected)
		if !ok || !Same(aged, value) {
			t.Fatal("Age did not preserve an unchanged immutable image")
		}
	}
	var sink Value
	if allocations := testing.AllocsPerRun(1_000, func() {
		var ok bool
		sink, ok = schema.Age(unchanged, selected)
		if !ok || !Same(sink, unchanged) {
			t.Fatal("Age no-op image")
		}
	}); allocations != 0 {
		t.Fatalf("Age unrelated-root allocations = %v, want 0", allocations)
	}
	if sink.IsBottom() {
		t.Fatal("Age allocation sink")
	}

	foreignLinked, foreignSchema := heapFixture(t, "heap_age_foreign_fence")
	foreign, _ := heapAgeAllocationKeys(t, foreignLinked, foreignSchema)
	if _, ok := schema.Age(foreignSchema.Top(), selected); ok {
		t.Fatal("Age admitted a foreign Value")
	}
	if _, ok := schema.Age(schema.Top(), foreign); ok {
		t.Fatal("Age admitted a foreign allocation key")
	}
	if _, ok := schema.Age(schema.Top(), Key{}); ok {
		t.Fatal("Age admitted an invalid key")
	}
}

// TestHeapAgeDeepReferenceTransportAndLaws proves the complete nested
// transform: metatable references, value/key containment and a Recent
// reference-key exception.  The latter must fold into the kind residual,
// because Summary is a whole-role fact and can never be an exact key atom.
func TestHeapAgeDeepReferenceTransportAndLaws(t *testing.T) {
	linked, schema := heapFixture(t, "heap_age_deep_laws")
	selected, other := heapAgeAllocationKeys(t, linked, schema)
	selectedRecent, selectedRecentOK := schema.Reference(selected, materialization.Recent)
	selectedSummary, selectedSummaryOK := schema.Reference(selected, materialization.Summary)
	otherRecent, otherRecentOK := schema.Reference(other, materialization.Recent)
	selectedContainment, selectedContainmentOK := schema.ContainmentExact(selectedRecent)
	otherContainment, otherContainmentOK := schema.ContainmentExact(otherRecent)
	if !selectedRecentOK || !selectedSummaryOK || !otherRecentOK || !selectedContainmentOK || !otherContainmentOK {
		t.Fatal("Age references")
	}
	slot, payload := heapAgeDynamicCandidate(t, schema)
	selector, selectorOK := schema.ReferenceSelector(selectedRecent)
	object, objectOK := schema.Object(ShapeEligible, FrozenMutable, selectedContainment)
	state, stateOK := schema.CellPresent(slot, payload, selectedContainment, otherContainment)
	if !selectorOK || !objectOK || !stateOK {
		t.Fatal("deep Age object")
	}
	object, objectOK = overwriteObjectCell(object, selector, state)
	world, worldOK := schema.One(selected, object)
	input, inputOK := schema.Relation(selected, world)
	if !objectOK || !worldOK || !inputOK {
		t.Fatal("deep Age relation")
	}
	before, beforeOK := schema.Fingerprint(input)
	aged, ageOK := schema.Age(input, selected)
	if !beforeOK || !ageOK || Same(aged, input) {
		t.Fatal("Age did not create a changed immutable image")
	}
	if after, ok := schema.Fingerprint(input); !ok || after != before {
		t.Fatal("Age mutated its predecessor")
	}
	inputWorld, inputWorldOK := input.WorldAt(0)
	inputObject, inputObjectOK := inputWorld.Recent()
	inputMeta, inputMetaOK := inputObject.MetatableAt(0)
	agedWorld, agedWorldOK := aged.WorldAt(0)
	agedObject, agedObjectOK := agedWorld.Recent()
	agedMeta, agedMetaOK := agedObject.MetatableAt(0)
	if !inputWorldOK || !inputObjectOK || !inputMetaOK || !agedWorldOK || !agedObjectOK || !agedMetaOK ||
		inputMeta != selectedRecent || agedMeta != selectedSummary {
		t.Fatal("Age did not preserve predecessor or transport metatable Recent")
	}
	if _, retained := agedObject.partition.exceptionIndex(selector.atoms[0]); retained {
		t.Fatal("Age retained an exact Summary reference-key exception")
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !keyAtomRuntimeKinds(schema.owner, selector.atoms[0]).Contains(kind) {
			continue
		}
		state := agedObject.partition.rest[kind]
		raw, rawOK := state.Raw()
		present, presentOK := state.PresentAt(0)
		valueChild, keyChild, childrenOK := present.Containment()
		valueReference, valueOK := valueChild.Reference()
		keyReference, keyOK := keyChild.Reference()
		if !rawOK || raw != rawAll || !presentOK || !childrenOK || !valueOK || !keyOK ||
			valueReference != selectedSummary || keyReference != otherRecent {
			t.Fatal("Age lost partition folding or rewrote an unrelated root")
		}
	}
	if summarySelector, ok := schema.ReferenceSelector(selectedSummary); ok || summarySelector.Valid() {
		t.Fatal("Age made Summary an exact key selector")
	}

	otherObject, otherObjectOK := schema.Object(ShapeEligible, FrozenMutable, otherContainment)
	otherWorld, otherWorldOK := schema.One(selected, otherObject)
	otherValue, otherValueOK := schema.Relation(selected, otherWorld)
	joined, joinedOK := Join(input, otherValue)
	if !otherObjectOK || !otherWorldOK || !otherValueOK || !joinedOK {
		t.Fatal("Age law representatives")
	}
	samples := []Value{schema.Bottom(), input, otherValue, joined, schema.Top()}
	for leftIndex, left := range samples {
		agedLeft, leftOK := schema.Age(left, selected)
		if !leftOK {
			t.Fatalf("Age representative %d", leftIndex)
		}
		for rightIndex, right := range samples {
			agedRight, rightOK := schema.Age(right, selected)
			if !rightOK {
				t.Fatalf("Age representative %d", rightIndex)
			}
			if LessOrEq(left, right) && !LessOrEq(agedLeft, agedRight) {
				t.Fatalf("Age is not monotone for %d <= %d", leftIndex, rightIndex)
			}
			union, unionOK := Join(left, right)
			agedUnion, agedUnionOK := schema.Age(union, selected)
			joinedAges, joinedAgesOK := Join(agedLeft, agedRight)
			if !unionOK || !agedUnionOK || !joinedAgesOK || !Equal(agedUnion, joinedAges) {
				t.Fatalf("Age does not preserve Join for %d/%d", leftIndex, rightIndex)
			}
		}
	}
	again, againOK := schema.Age(aged, selected)
	if !againOK || !Same(again, aged) {
		t.Fatal("Age is not idempotent as an immutable image")
	}
}

func heapAgeAllocationKeys(t testing.TB, linked *proglink.Link, schema Schema) (Key, Key) {
	t.Helper()
	selected, _, _ := allocationKeyWithField(t, schema)
	for index := 0; index < schema.KeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() != RootAllocation {
			continue
		}
		if key != selected {
			return selected, key
		}
	}
	t.Fatal("fixture omitted a second allocation root")
	return Key{}, Key{}
}

// heapAgeDynamicCandidate selects the global dynamic assignment that may
// inhabit the reference-key partition coordinate used by the deep Age
// fixture. Exact constructor-field slots are admissible only at their own
// exact-key atom.
func heapAgeDynamicCandidate(t testing.TB, schema Schema) (Slot, Payload) {
	t.Helper()
	for index := 0; index < schema.IndexAccessCount(); index++ {
		access, accessOK := schema.IndexAccessAt(index)
		if !accessOK {
			t.Fatal("index access")
		}
		slot, slotOK := schema.SlotForIndexAccess(access)
		payload, payloadOK := schema.PayloadForIndexAccess(access)
		kind, _, _, _, originOK := slot.Origin()
		if slotOK && payloadOK && originOK && kind == SlotDynamic {
			return slot, payload
		}
	}
	t.Fatal("fixture omitted a dynamic index assignment")
	return Slot{}, Payload{}
}
