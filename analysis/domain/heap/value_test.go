package heap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func heapFixture(t testing.TB, module string) (*proglink.Link, Schema) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: module + ".lua", Text: []byte(`
local child = { value = 1 }
local record = { child = child, ["name"] = child }
local key = "child"
record[key] = child
return record
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: module, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok {
		t.Fatal("Seal Heap")
	}
	return linked, schema
}

func allocationKeyWithField(t testing.TB, schema Schema) (Key, Slot, Payload) {
	t.Helper()
	for rootIndex := 0; rootIndex < schema.KeyCount(); rootIndex++ {
		key, ok := schema.KeyAt(rootIndex)
		if !ok || key.Kind() != RootAllocation {
			continue
		}
		for fieldIndex := 0; fieldIndex < schema.FieldCount(key); fieldIndex++ {
			field, ok := schema.FieldAt(key, fieldIndex)
			if !ok {
				t.Fatal("allocation field")
			}
			slot, slotOK := schema.SlotForField(field)
			payload, payloadOK := schema.PayloadForField(field)
			if slotOK && payloadOK {
				return key, slot, payload
			}
		}
	}
	t.Fatal("fixture omitted an allocation field")
	return Key{}, Slot{}, Payload{}
}

func mutableObject(t testing.TB, schema Schema) Object {
	t.Helper()
	object, ok := schema.Object(ShapeEligible, FrozenMutable, noneContainment(t, schema))
	if !ok {
		t.Fatal("mutable object")
	}
	return object
}

// TestHeapObjectSeedIssuesOnlyConcreteHeaders keeps the public construction
// boundary concrete. Header may-masks remain valid Objects, but only the
// canonical merge/Widen path may create them.
func TestHeapObjectSeedIssuesOnlyConcreteHeaders(t *testing.T) {
	_, schema := heapFixture(t, "heap_object_seed_headers")
	eligible, eligibleOK := schema.Object(ShapeEligible, FrozenMutable, noneContainment(t, schema))
	ineligible, ineligibleOK := schema.Object(ShapeIneligible, FrozenFrozen, noneContainment(t, schema))
	if !eligibleOK || !ineligibleOK {
		t.Fatal("singleton headers must seed objects")
	}
	if object, ok := schema.Object(ShapeEligible|ShapeIneligible, FrozenMutable, noneContainment(t, schema)); ok || object.Valid() {
		t.Fatal("seed constructor admitted a generalized Shape mask")
	}
	if object, ok := schema.Object(ShapeEligible, FrozenMutable|FrozenFrozen, noneContainment(t, schema)); ok || object.Valid() {
		t.Fatal("seed constructor admitted a generalized Frozen mask")
	}
	merged, mergedOK := mergeObjects(eligible, ineligible)
	shape, frozen, headerOK := merged.Header()
	if !mergedOK || !headerOK || shape != ShapeEligible|ShapeIneligible || frozen != FrozenMutable|FrozenFrozen {
		t.Fatal("canonical merge must retain its valid generalized header")
	}
}

func initializerPartitionState(t testing.TB, object Object, selector KeySelector) CellState {
	t.Helper()
	if !object.Valid() || !selector.valid() || selector.Kind() != KeySelectorAtom {
		t.Fatal("valid exact object coordinate")
	}
	state, ok := object.partition.lookup(selector.atoms[0])
	if !ok {
		t.Fatal("object partition coordinate")
	}
	return state
}

// TestHeapObjectInitializerExactWritesAreSourceOrdered proves that authored
// exact fields are a construction sequence, not a weak store sequence: the
// later duplicate entry replaces the earlier entry, and nil deletes it.
func TestHeapObjectInitializerExactWritesAreSourceOrdered(t *testing.T) {
	_, schema := heapFixture(t, "heap_initializer_exact")
	_, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	present := stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))
	absent, absentOK := schema.CellAbsent()
	if !absentOK {
		t.Fatal("absent cell")
	}
	initializer, initializerOK := schema.BeginObject(ShapeEligible, FrozenMutable, noneContainment(t, schema))
	if !initializerOK || !initializer.Apply(selector, present) {
		t.Fatal("exact present initialization")
	}
	first := initializerPartitionState(t, initializer.object, selector)
	if raw, ok := first.Raw(); !ok || raw != RawPresent {
		t.Fatal("exact present from absent must be definitely present")
	}
	if !initializer.Apply(selector, absent) {
		t.Fatal("exact duplicate delete")
	}
	object, objectOK := initializer.Finish()
	if !objectOK {
		t.Fatal("seal initialized object")
	}
	last := initializerPartitionState(t, object, selector)
	if raw, ok := last.Raw(); !ok || raw != RawAbsent || last.PresentCount() != 0 {
		t.Fatal("last exact authored entry must replace with RawAbsent")
	}
}

// TestHeapObjectInitializerDynamicKeyPreservesPriorPossibilities proves that
// an uncertain runtime key is not promoted to a strong update during object
// construction. Its observation joins every compatible prior coordinate.
func TestHeapObjectInitializerDynamicKeyPreservesPriorPossibilities(t *testing.T) {
	_, schema := heapFixture(t, "heap_initializer_dynamic")
	_, slot, payload := allocationKeyWithField(t, schema)
	exact := exactSelectorForSlot(t, schema, slot)
	present := stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))
	absent, absentOK := schema.CellAbsent()
	dynamic, dynamicOK := schema.KindSelector()
	if !absentOK || !dynamicOK {
		t.Fatal("dynamic initialization operands")
	}
	initializer, initializerOK := schema.BeginObject(ShapeEligible, FrozenMutable, noneContainment(t, schema))
	if !initializerOK || !initializer.Apply(exact, present) || !initializer.Apply(dynamic, absent) {
		t.Fatal("source-ordered dynamic initialization")
	}
	object, objectOK := initializer.Finish()
	if !objectOK {
		t.Fatal("seal dynamic initialized object")
	}
	state := initializerPartitionState(t, object, exact)
	if raw, ok := state.Raw(); !ok || raw != RawPresent|RawAbsent || state.PresentCount() != 1 {
		t.Fatal("dynamic selector erased an exact prior possibility")
	}
}

// TestHeapObjectInitializerIsOneShotAndPersistent proves that the mutable
// builder never aliases an earlier Object and cannot be reused after seal.
func TestHeapObjectInitializerIsOneShotAndPersistent(t *testing.T) {
	_, schema := heapFixture(t, "heap_initializer_oneshot")
	_, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	present := stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))
	absent, absentOK := schema.CellAbsent()
	if !absentOK {
		t.Fatal("absent cell")
	}
	initializer, initializerOK := schema.BeginObject(ShapeEligible, FrozenMutable, noneContainment(t, schema))
	if !initializerOK || !initializer.Apply(selector, present) {
		t.Fatal("first object state")
	}
	predecessor := initializer.object
	if !initializer.Apply(selector, absent) {
		t.Fatal("second object state")
	}
	object, objectOK := initializer.Finish()
	if !objectOK {
		t.Fatal("seal object")
	}
	before := initializerPartitionState(t, predecessor, selector)
	after := initializerPartitionState(t, object, selector)
	if raw, ok := before.Raw(); !ok || raw != RawPresent {
		t.Fatal("later initialization mutated predecessor object")
	}
	if raw, ok := after.Raw(); !ok || raw != RawAbsent {
		t.Fatal("sealed object missed final replacement")
	}
	if initializer.Apply(selector, present) {
		t.Fatal("sealed initializer accepted reuse")
	}
	if _, ok := initializer.Finish(); ok {
		t.Fatal("sealed initializer published twice")
	}
}

func TestHeapObjectInitializerCopiesBranchFromImmutablePrefix(t *testing.T) {
	_, schema := heapFixture(t, "heap_initializer_branches")
	_, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	present := stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))
	absent, absentOK := schema.CellAbsent()
	prefix, prefixOK := schema.BeginObject(ShapeEligible, FrozenMutable, noneContainment(t, schema))
	if !absentOK || !prefixOK || !prefix.Apply(selector, present) {
		t.Fatal("common initializer prefix")
	}
	deleted := prefix
	retained := prefix
	if !deleted.Apply(selector, absent) {
		t.Fatal("delete branch")
	}
	deletedObject, deletedOK := deleted.Finish()
	retainedObject, retainedOK := retained.Finish()
	prefixObject, prefixFinishOK := prefix.Finish()
	if !deletedOK || !retainedOK || !prefixFinishOK {
		t.Fatal("independent initializer branches")
	}
	deletedState := initializerPartitionState(t, deletedObject, selector)
	retainedState := initializerPartitionState(t, retainedObject, selector)
	prefixState := initializerPartitionState(t, prefixObject, selector)
	if raw, ok := deletedState.Raw(); !ok || raw != RawAbsent {
		t.Fatal("delete branch lost its replacement")
	}
	if raw, ok := retainedState.Raw(); !ok || raw != RawPresent {
		t.Fatal("delete branch mutated retained branch")
	}
	if raw, ok := prefixState.Raw(); !ok || raw != RawPresent {
		t.Fatal("branch consumption mutated the common prefix value")
	}
	if deleted.Apply(selector, present) || retained.Apply(selector, absent) || prefix.Apply(selector, absent) {
		t.Fatal("consumed initializer value accepted reuse")
	}
}

// TestHeapObjectInitializerRejectsForeignSchemaOperands keeps all mutable
// construction state inside the schema that issued it.
func TestHeapObjectInitializerRejectsForeignSchemaOperands(t *testing.T) {
	_, schema := heapFixture(t, "heap_initializer_owner")
	_, foreign := heapFixture(t, "heap_initializer_foreign")
	_, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	present := stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))
	foreignInitializer, foreignOK := foreign.BeginObject(ShapeEligible, FrozenMutable, noneContainment(t, foreign))
	if !foreignOK {
		t.Fatal("foreign initializer")
	}
	if foreignInitializer.Apply(selector, present) {
		t.Fatal("foreign schema initializer accepted operands")
	}
	foreignKey, _, _ := allocationKeyWithField(t, foreign)
	foreignReference, referenceOK := foreign.Reference(foreignKey, materialization.Recent)
	if !foreignKey.Valid() || !referenceOK {
		t.Fatal("foreign reference")
	}
	foreignExact := exactContainment(t, foreign, foreignReference)
	if _, ok := schema.BeginObject(ShapeEligible, FrozenMutable, foreignExact); ok {
		t.Fatal("initializer accepted foreign metatable")
	}
}

func stateForField(t testing.TB, schema Schema, slot Slot, payload Payload, valueChild, keyChild Containment) CellState {
	t.Helper()
	state, ok := schema.CellPresent(slot, payload, valueChild, keyChild)
	if !ok {
		t.Fatal("present state")
	}
	return state
}

func noneContainment(t testing.TB, schema Schema) Containment {
	t.Helper()
	containment, ok := schema.ContainmentNone()
	if !ok {
		t.Fatal("None containment")
	}
	return containment
}

func exactContainment(t testing.TB, schema Schema, reference Reference) Containment {
	t.Helper()
	containment, ok := schema.ContainmentExact(reference)
	if !ok {
		t.Fatal("Exact containment")
	}
	return containment
}

func valueWithFieldContainment(t testing.TB, schema Schema, key Key, selector KeySelector, slot Slot, payload Payload, containment Containment) Value {
	t.Helper()
	object, ok := overwriteObjectCell(mutableObject(t, schema), selector, stateForField(t, schema, slot, payload, containment, noneContainment(t, schema)))
	if !ok {
		t.Fatal("object with contained field")
	}
	world, worldOK := schema.One(key, object)
	value, valueOK := schema.Relation(key, world)
	if !worldOK || !valueOK {
		t.Fatal("value with contained field")
	}
	return value
}

func valueHasContainmentKind(value Value, selector KeySelector, want ContainmentKind) bool {
	for _, world := range value.worlds {
		var objects []Object
		switch world.kind {
		case WorldExact:
			objects = []Object{world.exact}
		case WorldOne:
			objects = []Object{world.recent}
		case WorldMany:
			objects = []Object{world.recent, world.summary}
		}
		for _, object := range objects {
			state, ok := object.partition.lookup(selector.atoms[0])
			if !ok {
				continue
			}
			for _, present := range state.presents {
				if present.valueContainment.kind == want {
					return true
				}
			}
		}
	}
	return false
}

func TestHeapContainmentCarrierDistinguishesAndPreservesOpaqueEdges(t *testing.T) {
	_, schema := heapFixture(t, "heap_containment_carrier")
	key, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	none, noneOK := schema.ContainmentNone()
	unknown, unknownOK := schema.ContainmentUnknown()
	recent, recentOK := schema.Reference(key, materialization.Recent)
	exact, exactOK := schema.ContainmentExact(recent)
	if !noneOK || !unknownOK || !recentOK || !exactOK || !none.Valid() || !unknown.Valid() || !exact.Valid() {
		t.Fatal("schema did not issue all containment families")
	}
	if none.Kind() != ContainmentNone || unknown.Kind() != ContainmentUnknown || exact.Kind() != ContainmentExact {
		t.Fatal("containment kind projection")
	}
	if _, claimed := none.Reference(); claimed {
		t.Fatal("None made an exact-reference claim")
	}
	if _, claimed := unknown.Reference(); claimed {
		t.Fatal("Unknown made an exact-reference claim")
	}
	if projected, claimed := exact.Reference(); !claimed || projected != recent {
		t.Fatal("Exact lost its structural reference")
	}

	_, foreignSchema := heapFixture(t, "heap_containment_foreign")
	foreignKey, _, _ := allocationKeyWithField(t, foreignSchema)
	foreignReference, foreignReferenceOK := foreignSchema.Reference(foreignKey, materialization.Recent)
	foreignNone, foreignNoneOK := foreignSchema.ContainmentNone()
	if !foreignReferenceOK || !foreignNoneOK {
		t.Fatal("foreign containment fixture")
	}
	if containment, ok := schema.ContainmentExact(Reference{}); ok || containment.Valid() {
		t.Fatal("zero reference was admitted as Exact")
	}
	if containment, ok := schema.ContainmentExact(foreignReference); ok || containment.Valid() {
		t.Fatal("foreign reference was admitted as Exact")
	}
	if state, ok := schema.CellPresent(slot, payload, Containment{}, none); ok || state.Valid() {
		t.Fatal("zero containment was admitted")
	}
	if state, ok := schema.CellPresent(slot, payload, foreignNone, none); ok || state.Valid() {
		t.Fatal("foreign containment was admitted")
	}

	noneState := stateForField(t, schema, slot, payload, none, none)
	unknownState := stateForField(t, schema, slot, payload, unknown, none)
	if equalCellState(noneState, unknownState) {
		t.Fatal("None and Unknown collapsed to one present tuple")
	}
	joinedState, joinedStateOK := mergeCellStates(noneState, unknownState)
	if !joinedStateOK || joinedState.PresentCount() != 2 {
		t.Fatal("cell join lost the opaque edge alternative")
	}

	noneValue := valueWithFieldContainment(t, schema, key, selector, slot, payload, none)
	unknownValueRelation := valueWithFieldContainment(t, schema, key, selector, slot, payload, unknown)
	agedUnknown, ageOK := schema.Age(unknownValueRelation, key)
	if !ageOK || !Same(agedUnknown, unknownValueRelation) {
		t.Fatal("Age rewrote Unknown containment")
	}
	noneFingerprint, noneFingerprintOK := schema.Fingerprint(noneValue)
	unknownFingerprint, unknownFingerprintOK := schema.Fingerprint(unknownValueRelation)
	if !noneFingerprintOK || !unknownFingerprintOK || noneFingerprint == unknownFingerprint {
		t.Fatal("fingerprint collapsed None and Unknown")
	}
	joined, joinOK := Join(noneValue, unknownValueRelation)
	widened, widenOK := Widen(noneValue, unknownValueRelation)
	if !joinOK || !widenOK || !valueHasContainmentKind(joined, selector, ContainmentUnknown) || !valueHasContainmentKind(widened, selector, ContainmentUnknown) {
		t.Fatal("join or widening lost Unknown containment")
	}
	rank, rankOK := NewWidenRank(schema)
	if !rankOK || rank.Width() != 3 || rank.At(key, noneValue, 0) == 0 || rank.At(key, unknownValueRelation, 0) == 0 || rank.At(key, widened, 0) == 0 {
		t.Fatal("sealed containment carrier lacks a total widening rank")
	}
}

func TestHeapMetatableContainmentHasOneExplicitThreeStatePath(t *testing.T) {
	_, schema := heapFixture(t, "heap_metatable_containment")
	key, _, _ := allocationKeyWithField(t, schema)
	none := noneContainment(t, schema)
	unknown, unknownOK := schema.ContainmentUnknown()
	recent, recentOK := schema.Reference(key, materialization.Recent)
	summary, summaryOK := schema.Reference(key, materialization.Summary)
	exact := exactContainment(t, schema, recent)
	summaryExact := exactContainment(t, schema, summary)
	if !unknownOK || !recentOK || !summaryOK {
		t.Fatal("metatable containment fixture")
	}
	noneObject, noneObjectOK := schema.Object(ShapeEligible, FrozenMutable, none)
	unknownObject, unknownObjectOK := schema.Object(ShapeEligible, FrozenMutable, unknown)
	exactObject, exactObjectOK := schema.Object(ShapeEligible, FrozenMutable, exact)
	if !noneObjectOK || !unknownObjectOK || !exactObjectOK {
		t.Fatal("explicit metatable object seeds")
	}
	if !noneObject.MayHaveNoMetatable() || noneObject.MayHaveUnknownMetatable() || noneObject.MetatableCount() != 0 {
		t.Fatal("None metatable seed")
	}
	if unknownObject.MayHaveNoMetatable() || !unknownObject.MayHaveUnknownMetatable() || unknownObject.MetatableCount() != 0 {
		t.Fatal("Unknown metatable seed collapsed with None or Exact")
	}
	if exactObject.MayHaveNoMetatable() || exactObject.MayHaveUnknownMetatable() || exactObject.MetatableCount() != 1 {
		t.Fatal("Exact metatable seed")
	}

	coexisting, coexistOK := mergeObjects(noneObject, exactObject)
	coexisting, coexistUnknownOK := mergeObjects(coexisting, unknownObject)
	if !coexistOK || !coexistUnknownOK || !coexisting.MayHaveNoMetatable() || !coexisting.MayHaveUnknownMetatable() || coexisting.MetatableCount() != 1 {
		t.Fatal("metatable LUB did not retain None, Unknown, and Exact")
	}
	summaryObject, summaryObjectOK := schema.Object(ShapeEligible, FrozenMutable, summaryExact)
	expected, expectedOK := mergeObjects(noneObject, summaryObject)
	expected, expectedUnknownOK := mergeObjects(expected, unknownObject)
	coexistingWorld, coexistingWorldOK := schema.One(key, coexisting)
	coexistingValue, coexistingValueOK := schema.Relation(key, coexistingWorld)
	expectedWorld, expectedWorldOK := schema.One(key, expected)
	expectedValue, expectedValueOK := schema.Relation(key, expectedWorld)
	aged, ageOK := schema.Age(coexistingValue, key)
	if !summaryObjectOK || !expectedOK || !expectedUnknownOK || !coexistingWorldOK || !coexistingValueOK || !expectedWorldOK || !expectedValueOK || !ageOK || !Equal(aged, expectedValue) {
		t.Fatal("Age did not preserve None and Unknown metatables while advancing Exact")
	}

	noneWorld, noneWorldOK := schema.One(key, noneObject)
	unknownWorld, unknownWorldOK := schema.One(key, unknownObject)
	noneValue, noneValueOK := schema.Relation(key, noneWorld)
	unknownValue, unknownValueOK := schema.Relation(key, unknownWorld)
	noneFingerprint, noneFingerprintOK := schema.Fingerprint(noneValue)
	unknownFingerprint, unknownFingerprintOK := schema.Fingerprint(unknownValue)
	if !noneWorldOK || !unknownWorldOK || !noneValueOK || !unknownValueOK || Equal(noneValue, unknownValue) ||
		!noneFingerprintOK || !unknownFingerprintOK || noneFingerprint == unknownFingerprint {
		t.Fatal("None and Unknown metatables are not distinct in equality/fingerprint")
	}

	_, foreignSchema := heapFixture(t, "heap_metatable_foreign")
	foreignKey, _, _ := allocationKeyWithField(t, foreignSchema)
	foreignReference, foreignReferenceOK := foreignSchema.Reference(foreignKey, materialization.Recent)
	foreignExact := exactContainment(t, foreignSchema, foreignReference)
	if !foreignReferenceOK {
		t.Fatal("foreign metatable fixture")
	}
	if object, ok := schema.Object(ShapeEligible, FrozenMutable, Containment{}); ok || object.Valid() {
		t.Fatal("Object accepted invalid metatable containment")
	}
	if object, ok := schema.Object(ShapeEligible, FrozenMutable, foreignExact); ok || object.Valid() {
		t.Fatal("Object accepted foreign metatable containment")
	}
	if initializer, ok := schema.BeginObject(ShapeEligible, FrozenMutable, foreignExact); ok || initializer.owner != nil {
		t.Fatal("BeginObject accepted foreign metatable containment")
	}
}

func exactSelectorForSlot(t testing.TB, schema Schema, slot Slot) KeySelector {
	t.Helper()
	selector, ok := schema.SelectorForSlot(slot)
	if !ok || selector.Kind() != KeySelectorAtom || len(selector.atoms) != 1 {
		t.Fatal("exact field selector")
	}
	return selector
}

func TestHeapPartitionIsCompleteAndExactUpdateSplitsOnlyItsAtom(t *testing.T) {
	_, schema := heapFixture(t, "heap_partition_split")
	_, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	object := mutableObject(t, schema)
	updated, ok := overwriteObjectCell(object, selector, stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema)))
	if !ok || !updated.partition.valid() {
		t.Fatal("exact update did not produce a canonical partition")
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		raw, rawOK := updated.partition.rest[kind].Raw()
		if !rawOK || raw != RawAbsent {
			t.Fatalf("residual %v changed during exact split: %v/%v", kind, raw, rawOK)
		}
	}
	if len(updated.partition.exceptions) != 1 || compareKeyAtom(updated.partition.exceptions[0].atom, selector.atoms[0]) != 0 {
		t.Fatal("exact update did not create one atomic exception")
	}
	raw, rawOK := updated.partition.exceptions[0].state.Raw()
	if !rawOK || raw != RawPresent {
		t.Fatal("exact exception lost present state")
	}
}

func TestHeapSelectorsNeverStoreSourceIdentity(t *testing.T) {
	linked, schema := heapFixture(t, "heap_selectors")
	var dynamic Slot
	for index := 0; index < schema.SlotCount(); index++ {
		slot, ok := schema.SlotAt(index)
		if !ok {
			t.Fatal("slot")
		}
		kind, _, _, _, _ := slot.Origin()
		if kind == SlotDynamic {
			dynamic = slot
			break
		}
	}
	if !dynamic.valid() {
		t.Fatal("fixture omitted dynamic access")
	}
	selector, ok := schema.SelectorForSlot(dynamic)
	if !ok || selector.Kind() != KeySelectorKinds || selector.ExactCount() != 0 || selector.ReferenceCount() != 0 || selector.RuntimeKinds()&runtimekind.Bit(runtimekind.Nil) != 0 {
		t.Fatal("dynamic source became a stored equality identity")
	}
	keys := linked.Project().Keys()
	if keys.Count() < 2 {
		t.Fatal("fixture omitted exact Link keys")
	}
	first, firstOK := keys.At(0)
	second, secondOK := keys.At(1)
	left, leftOK := schema.ExactSelector(first)
	right, rightOK := schema.ExactSelector(second)
	finite, finiteOK := schema.FiniteSelector(left, right)
	if !firstOK || !secondOK || !leftOK || !rightOK || !finiteOK || finite.Kind() != KeySelectorFinite || finite.ExactCount() != 2 {
		t.Fatal("finite exact selector was not normalized")
	}
}

func TestHeapRawAbsenceHasNoPayloadTuple(t *testing.T) {
	_, schema := heapFixture(t, "heap_raw")
	_, slot, payload := allocationKeyWithField(t, schema)
	absent, ok := schema.CellAbsent()
	if !ok {
		t.Fatal("absent state")
	}
	raw, rawOK := absent.Raw()
	if !rawOK || raw != RawAbsent || absent.PresentCount() != 0 {
		t.Fatal("RawAbsent retained present payload state")
	}
	present := stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema))
	if present.PresentCount() != 1 {
		t.Fatal("RawPresent omitted its complete tuple")
	}
	entry, ok := present.PresentAt(0)
	if !ok {
		t.Fatal("present tuple")
	}
	if source, ok := entry.Slot(); !ok || source != slot {
		t.Fatal("present tuple lost cold slot provenance")
	}
}

func TestHeapKindWriteUpdatesResidualAndExceptionWithoutErasure(t *testing.T) {
	_, schema := heapFixture(t, "heap_kind_write")
	_, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	object, ok := overwriteObjectCell(mutableObject(t, schema), selector, stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema)))
	if !ok {
		t.Fatal("exact setup")
	}
	kinds, ok := schema.KindSelector()
	if !ok {
		t.Fatal("all-kind selector")
	}
	absent, _ := schema.CellAbsent()
	updated, ok := weakObjectCell(object, kinds, absent)
	if !ok || !updated.partition.valid() {
		t.Fatal("weak kind update")
	}
	atom := selector.atoms[0]
	index, retained := updated.partition.exceptionIndex(atom)
	if !retained {
		t.Fatal("kind update erased a non-default exact exception")
	}
	raw, rawOK := updated.partition.exceptions[index].state.Raw()
	if !rawOK || raw != rawAll {
		t.Fatal("kind update did not join the selected exact state")
	}
	for kind := range runtimekind.Count {
		if !legalKeyKind(runtimekind.Kind(kind)) || !keyAtomRuntimeKinds(schema.owner, atom).Contains(runtimekind.Kind(kind)) {
			continue
		}
		raw, rawOK = updated.partition.rest[kind].Raw()
		if !rawOK || raw != RawAbsent {
			t.Fatal("kind residual was not preserved around the exception")
		}
	}
}

func TestHeapPartitionJoinIsPointwiseAndKeepsOneSidedException(t *testing.T) {
	_, schema := heapFixture(t, "heap_partition_join")
	_, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	left, ok := overwriteObjectCell(mutableObject(t, schema), selector, stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema)))
	if !ok {
		t.Fatal("left exact state")
	}
	right := mutableObject(t, schema)
	joined, ok := mergeObjects(left, right)
	if !ok || !joined.partition.valid() {
		t.Fatal("pointwise partition join")
	}
	atom := selector.atoms[0]
	index, retained := joined.partition.exceptionIndex(atom)
	if !retained {
		t.Fatal("one-sided exact exception was folded without equality")
	}
	raw, rawOK := joined.partition.exceptions[index].state.Raw()
	if !rawOK || raw != rawAll {
		t.Fatal("pointwise exception join lost absent/present alternatives")
	}
}

// TestHeapCreateDoesNotMutatePredecessor proves that the principal exact
// constructor builds a successor without mutating a retained predecessor.
func TestHeapCreateDoesNotMutatePredecessor(t *testing.T) {
	_, schema := heapFixture(t, "heap_create_immutable")
	key, _, _ := allocationKeyWithField(t, schema)
	recent, recentOK := schema.Reference(key, materialization.Recent)
	summary, summaryOK := schema.Reference(key, materialization.Summary)
	predecessorObject, objectOK := schema.Object(ShapeEligible, FrozenMutable, exactContainment(t, schema, recent))
	predecessorWorld, worldOK := schema.One(key, predecessorObject)
	predecessor, predecessorOK := schema.Relation(key, predecessorWorld)
	if !recentOK || !summaryOK || !objectOK || !worldOK || !predecessorOK {
		t.Fatal("Create predecessor")
	}
	fingerprint, fingerprintOK := schema.Fingerprint(predecessor)
	if !fingerprintOK {
		t.Fatal("Create predecessor fingerprint")
	}
	created, createdOK := schema.Create(predecessor, key, mutableObject(t, schema))
	if !createdOK {
		t.Fatal("Create")
	}
	assertOneWorldMetatable(t, schema, predecessor, fingerprint, recent)
	createdWorld, createdWorldOK := created.WorldAt(0)
	createdSummary, createdSummaryOK := createdWorld.Summary()
	createdMetatable, createdMetatableOK := createdSummary.MetatableAt(0)
	if !createdWorldOK || !createdSummaryOK || !createdMetatableOK || createdMetatable != summary {
		t.Fatal("Create did not age the predecessor into the successor summary")
	}
}

func assertOneWorldMetatable(t testing.TB, schema Schema, predecessor Value, wantFingerprint uint64, want Reference) {
	t.Helper()
	fingerprint, fingerprintOK := schema.Fingerprint(predecessor)
	world, worldOK := predecessor.WorldAt(0)
	object, objectOK := world.Recent()
	metatable, metatableOK := object.MetatableAt(0)
	if !fingerprintOK || fingerprint != wantFingerprint || !worldOK || !objectOK || !metatableOK || metatable != want {
		t.Fatal("Create mutated its predecessor")
	}
}

func TestHeapRelationCanonicalizesDominatedCompleteWorlds(t *testing.T) {
	_, schema := heapFixture(t, "heap_relation_antichain")
	key, slot, payload := allocationKeyWithField(t, schema)
	selector := exactSelectorForSlot(t, schema, slot)
	small := mutableObject(t, schema)
	large, ok := weakObjectCell(small, selector, stateForField(t, schema, slot, payload, noneContainment(t, schema), noneContainment(t, schema)))
	if !ok || !objectLessOrEq(small, large) {
		t.Fatal("dominating object setup")
	}
	left, leftOK := schema.One(key, small)
	right, rightOK := schema.One(key, large)
	value, valueOK := schema.Relation(key, left, right)
	if !leftOK || !rightOK || !valueOK || value.WorldCount() != 1 {
		t.Fatal("Relation retained a dominated complete world")
	}
}
