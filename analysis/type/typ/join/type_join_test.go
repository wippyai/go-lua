package join

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typ/unwrap"
)

func TestTypes_Empty(t *testing.T) {
	result := Types()
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestTypes_Single(t *testing.T) {
	result := Types(typ.String)
	if result != typ.String {
		t.Errorf("expected string, got %v", result)
	}
}

func TestTypes_AllEqual(t *testing.T) {
	result := Types(typ.String, typ.String, typ.String)
	if result != typ.String {
		t.Errorf("expected string, got %v", result)
	}
}

type countingCompoundJoinType struct {
	name      string
	hashCalls *int
}

func (t *countingCompoundJoinType) Kind() kind.Kind { return kind.Record }
func (t *countingCompoundJoinType) String() string  { return t.name }
func (t *countingCompoundJoinType) Hash() uint64 {
	if t.hashCalls != nil {
		*t.hashCalls = *t.hashCalls + 1
	}
	return 1
}
func (t *countingCompoundJoinType) Equals(other typ.Type) bool { return t == other }

func TestDedupeJoinInputsCompoundValuesUseIdentityWithoutHashing(t *testing.T) {
	hashCalls := 0
	a := &countingCompoundJoinType{name: "a", hashCalls: &hashCalls}
	b := &countingCompoundJoinType{name: "b", hashCalls: &hashCalls}

	got := dedupeJoinInputs([]typ.Type{a, b, a})
	if len(got) != 2 {
		t.Fatalf("dedupeJoinInputs compound len = %d, want 2", len(got))
	}
	if hashCalls != 0 {
		t.Fatalf("compound join dedupe called Hash %d times, want 0", hashCalls)
	}
}

func TestFlattenUnionsCompoundLeavesUseIdentityWithoutHashing(t *testing.T) {
	hashCalls := 0
	compound := &countingCompoundJoinType{name: "compound", hashCalls: &hashCalls}
	nested := typ.NewUnion(compound, typ.String)
	hashCalls = 0

	got := FlattenUnions([]typ.Type{nested, nested})
	if len(got) != 2 {
		t.Fatalf("FlattenUnions len = %d, want compound and string once", len(got))
	}
	if hashCalls != 0 {
		t.Fatalf("FlattenUnions called compound Hash %d times, want 0", hashCalls)
	}
}

func TestTypesDeduplicatesEquivalentRecursiveProductFamilies(t *testing.T) {
	left := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			OptField("next", self).
			Build()
	})
	right := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			OptField("next", self).
			Build()
	})

	got := Types(left, right)
	if got != left {
		t.Fatalf("Types(equivalent recursive products) = %T %[1]v, want first member", got)
	}
}

func TestTypes_Different(t *testing.T) {
	result := Types(typ.String, typ.Number)
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(u.Members))
	}
}

func TestCoalesceMaps_NoMaps(t *testing.T) {
	input := []typ.Type{typ.String, typ.Number}
	result := CoalesceMaps(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types, got %d", len(result))
	}
}

func TestCoalesceMaps_SingleMap(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	input := []typ.Type{m, typ.Boolean}
	result := CoalesceMaps(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types (no coalescing), got %d", len(result))
	}
}

func TestCoalesceMaps_MultipleMaps(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.Integer, typ.Boolean)
	input := []typ.Type{m1, m2}
	result := CoalesceMaps(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}
	m, ok := result[0].(*typ.Map)
	if !ok {
		t.Fatalf("expected map, got %T", result[0])
	}
	// Key should be string | integer
	if _, ok := m.Key.(*typ.Union); !ok {
		t.Errorf("expected union key type, got %T", m.Key)
	}
	// Value should be number | boolean
	if _, ok := m.Value.(*typ.Union); !ok {
		t.Errorf("expected union value type, got %T", m.Value)
	}
}

func TestTypesCoalescesLargeMapValueFamilyInBatch(t *testing.T) {
	maps := make([]typ.Type, 0, 512)
	for i := 0; i < cap(maps); i++ {
		maps = append(maps, typ.NewMap(typ.String, typ.NewRecord().
			Field("index", typ.LiteralInt(int64(i))).
			Field("name", typ.LiteralString("case")).
			Build()))
	}

	got := Types(maps...)
	m, ok := got.(*typ.Map)
	if !ok {
		t.Fatalf("Types(large map family) = %T %[1]v, want map", got)
	}
	rec, ok := m.Value.(*typ.Record)
	if !ok {
		t.Fatalf("coalesced map value = %T %[1]v, want record", m.Value)
	}
	index := rec.GetField("index")
	if index == nil || !typ.TypeEquals(index.Type, typ.Integer) {
		t.Fatalf("coalesced index field = %v, want integer", index)
	}
}

func TestTypesPrunesRefinableMapWhenPreciseRecursiveProductExists(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	soft := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	precise := typ.NewRecursive("Flow", func(self typ.Type) typ.Type {
		return typ.NewMap(typ.String, typ.NewArray(entry))
	})

	got := Types(soft, precise)
	if !typ.TypeEquals(got, precise) {
		t.Fatalf("Types(soft map, precise recursive map) = %v, want %v", got, precise)
	}
}

func TestTypes_CoalescesRecordMapComponentsThroughCompatibleRecordLaw(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.String).Build()).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.String).Build()).
		MapComponent(typ.String, typ.Boolean).
		Build()

	got := Types(left, right)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("Types(record map components) = %T %[1]v, want record", got)
	}
	if !rec.HasMapComponent() {
		t.Fatalf("coalesced record should keep map component: %v", rec)
	}
	if _, ok := rec.MapValue.(*typ.Union); !ok {
		t.Fatalf("expected merged map value union, got %T", rec.MapValue)
	}
}

func TestTypes_CoalescesRecordMapFamilyWithOptionalFields(t *testing.T) {
	left := typ.NewRecord().
		Field("suite", typ.String).
		Field("started", typ.Boolean).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("suite", typ.String).
		Field("finished", typ.Boolean).
		MapComponent(typ.String, typ.Boolean).
		Build()

	got := Types(left, right)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("Types(record map family) = %T %[1]v, want record", got)
	}
	for _, name := range []string{"started", "finished"} {
		field := rec.GetField(name)
		if field == nil || !field.Optional {
			t.Fatalf("field %q should be optional after family coalescing: %v", name, field)
		}
	}
	if _, ok := rec.MapValue.(*typ.Union); !ok {
		t.Fatalf("expected merged map value union, got %T", rec.MapValue)
	}
}

func TestTypes_RecordMapFamilyKeepsSingleBranchFieldOptionalWhenMapValueMatches(t *testing.T) {
	message := typ.NewRecord().
		Field("topic", typ.Func().Returns(typ.String).Build()).
		Build()
	left := typ.NewRecord().
		Field("root", message).
		MapComponent(typ.String, message).
		Build()
	right := typ.NewRecord().
		MapComponent(typ.String, message).
		Build()

	got := Types(left, right)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("Types(record map family) = %T %[1]v, want record", got)
	}
	field := rec.GetField("root")
	if field == nil {
		t.Fatalf("expected root field in coalesced record: %v", rec)
	}
	if !field.Optional {
		t.Fatalf("root exists on one operand only, so it must remain optional: %v", field)
	}
}

func TestTypes_RecordMapFamilyPreservesConflictingDiscriminants(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.LiteralString("start")).
		Field("payload", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.LiteralString("stop")).
		Field("payload", typ.Integer).
		MapComponent(typ.String, typ.Number).
		Build()

	got := Types(left, right)
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("Types(discriminated record map family) = %T %[1]v, want union", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("discriminated record map family members = %d, want 2", len(union.Members))
	}
}

func TestCoalesceEmptyRecordWithMap_NoEmptyRecord(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	input := []typ.Type{m, rec}
	result := CoalesceEmptyRecordWithMap(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types, got %d", len(result))
	}
}

func TestCoalesceEmptyRecordWithMap_NoMap(t *testing.T) {
	rec := typ.NewRecord().Build()
	input := []typ.Type{typ.String, rec}
	result := CoalesceEmptyRecordWithMap(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types, got %d", len(result))
	}
}

func TestCoalesceEmptyRecordWithMap_BothPresent(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	rec := typ.NewRecord().Build()
	input := []typ.Type{m, rec}
	result := CoalesceEmptyRecordWithMap(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 type (empty record removed), got %d", len(result))
	}
	if _, ok := result[0].(*typ.Map); !ok {
		t.Errorf("expected map to remain, got %T", result[0])
	}
}

func TestCoalesceEmptyRecordWithArray_BothPresent(t *testing.T) {
	arr := typ.NewArray(typ.Number)
	rec := typ.NewRecord().Build()
	input := []typ.Type{arr, rec}
	result := CoalesceEmptyRecordWithArray(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 type (empty record removed), got %d", len(result))
	}
	if _, ok := result[0].(*typ.Array); !ok {
		t.Errorf("expected array to remain, got %T", result[0])
	}
}

func TestIsEmptyRecord_Nil(t *testing.T) {
	if unwrap.IsEmptyRecord(nil) {
		t.Error("nil should not be empty record")
	}
}

func TestIsEmptyRecord_NonRecord(t *testing.T) {
	if unwrap.IsEmptyRecord(typ.String) {
		t.Error("string should not be empty record")
	}
}

func TestIsEmptyRecord_EmptyRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	if !unwrap.IsEmptyRecord(rec) {
		t.Error("empty record should be empty record")
	}
}

func TestIsEmptyRecord_NonEmptyRecord(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	if unwrap.IsEmptyRecord(rec) {
		t.Error("record with fields should not be empty record")
	}
}

func TestTypes_IntegrationWithCoalescing(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.String, typ.Boolean)
	emptyRec := typ.NewRecord().Build()

	result := Types(m1, m2, emptyRec)

	// Should coalesce maps and remove empty record
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("expected single map after coalescing, got %T", result)
	}
	if m.Key != typ.String {
		t.Errorf("expected string key, got %v", m.Key)
	}
	// Value should be number | boolean
	if _, ok := m.Value.(*typ.Union); !ok {
		t.Errorf("expected union value type, got %T", m.Value)
	}
}

func TestTypes_IntegrationWithArrayCoalescing(t *testing.T) {
	arr := typ.NewArray(typ.String)
	emptyRec := typ.NewRecord().Build()

	result := Types(arr, emptyRec)
	gotArr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected array after coalescing, got %T", result)
	}
	if !typ.TypeEquals(gotArr.Element, typ.String) {
		t.Fatalf("expected string[] element, got %v", gotArr.Element)
	}
}

func TestTypes_Idempotence(t *testing.T) {
	// join(x, y) repeated should yield TypeEquals true
	j1 := Types(typ.Number, typ.String)
	j2 := Types(typ.Number, typ.String)

	if !typ.TypeEquals(j1, j2) {
		t.Error("repeated joins should be equal")
	}

	if j1.Hash() != j2.Hash() {
		t.Error("repeated joins should have same hash")
	}
}

func TestTypes_IdempotenceNested(t *testing.T) {
	// join(join(A, B), A) == join(A, B)
	j1 := Types(typ.Number, typ.String)
	j2 := Types(j1, typ.Number)

	if !typ.TypeEquals(j1, j2) {
		t.Error("adding existing member via join should not change result")
	}
}

func TestTypes_OrderIndependence(t *testing.T) {
	j1 := Types(typ.Number, typ.String, typ.Boolean)
	j2 := Types(typ.Boolean, typ.Number, typ.String)
	j3 := Types(typ.String, typ.Boolean, typ.Number)

	if !typ.TypeEquals(j1, j2) {
		t.Error("j1 should equal j2")
	}
	if !typ.TypeEquals(j2, j3) {
		t.Error("j2 should equal j3")
	}
}

func TestTypes_WithUnionInput(t *testing.T) {
	// Joining with an existing union should flatten
	u := typ.NewUnion(typ.Number, typ.String)
	j := Types(u, typ.Boolean)

	result, ok := j.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", j)
	}

	if len(result.Members) != 3 {
		t.Errorf("expected 3 members, got %d", len(result.Members))
	}
}

func TestFlattenUnionsDeduplicatesRepeatedMembers(t *testing.T) {
	member := typ.NewRecord().Field("id", typ.String).Build()
	u := typ.NewUnion(member, typ.Number)
	flat := FlattenUnions([]typ.Type{u, u, member})

	count := 0
	for _, t := range flat {
		if typ.TypeEquals(t, member) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("flattened repeated member count = %d, want 1 in %v", count, flat)
	}
}

func TestTypes_AllUnknown(t *testing.T) {
	result := Types(typ.Unknown, typ.Unknown, typ.Unknown)
	if result != typ.Unknown {
		t.Errorf("all unknown should return unknown, got %v", result)
	}
}

func TestTypes_MixedUnknown(t *testing.T) {
	result := Types(typ.String, typ.Unknown, typ.Number)
	// Unknown should be filtered out
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	// Should have string and number only
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members after filtering unknown, got %d", len(u.Members))
	}
}

func TestTypes_CoalescesCompatibleRecursiveRecordFamilies(t *testing.T) {
	base := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	withPath := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	got := Types(base, withPath)
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("Types(recursive family) = %T %[1]v, want recursive", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T, want record", rec.Body)
	}
	fullPath := body.GetField("full_path")
	if fullPath == nil || !fullPath.Optional || !typ.TypeEquals(fullPath.Type, typ.String) {
		t.Fatalf("full_path field = %v, want optional string", fullPath)
	}
	children := body.GetField("children")
	if children == nil {
		t.Fatalf("missing children field in %v", body)
	}
	arr, ok := children.Type.(*typ.Array)
	if !ok || !typ.IsRecursiveRef(arr.Element, rec) {
		t.Fatalf("children field = %v, want array of merged recursive self", children.Type)
	}
}

func TestTypes_PreservesDiscriminatedRecursiveRecordFamilies(t *testing.T) {
	a := typ.NewRecursive("Case", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("a")).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	b := typ.NewRecursive("Case", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("b")).
			Field("next", typ.NewOptional(self)).
			Build()
	})

	got := Types(a, b)
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("Types(discriminated recursive family) = %T %[1]v, want union", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("discriminated recursive union members = %d, want 2", len(union.Members))
	}
}

func TestTypes_CoalescesRecursiveRecordFamiliesByBodyNotName(t *testing.T) {
	left := typ.NewRecursive("Flow", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Inferred", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("parent", typ.NewOptional(self)).
			Build()
	})

	got := Types(left, right)
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("Types(different-name recursive family) = %T %[1]v, want recursive", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T, want record", rec.Body)
	}
	parent := body.GetField("parent")
	if parent == nil || !parent.Optional {
		t.Fatalf("parent field = %v, want optional recursive field", parent)
	}
}

func TestTypes_IncrementalJoinSeesFlattenedRecursiveFamilies(t *testing.T) {
	a := typ.NewRecursive("FlowA", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	b := typ.NewRecursive("FlowB", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})
	c := typ.NewRecursive("FlowC", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("parent", typ.NewOptional(self)).
			Build()
	})

	batch := Types(a, b, c)
	incremental := Types(Types(a, b), c)
	if !typ.TypeEquals(batch, incremental) {
		t.Fatalf("incremental join = %v, batch = %v", incremental, batch)
	}
	if _, ok := incremental.(*typ.Recursive); !ok {
		t.Fatalf("incremental join = %T %[1]v, want recursive", incremental)
	}
}

func TestTypes_RecursiveFamilyJoinStableAcrossOrderAndRejoin(t *testing.T) {
	a := typ.NewRecursive("FlowA", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	b := typ.NewRecursive("FlowB", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	ab := Types(a, b)
	ba := Types(b, a)
	if !typ.TypeEquals(ab, ba) {
		t.Fatalf("recursive join differs by input order: %v vs %v", ab, ba)
	}
	if ab.Hash() != ba.Hash() {
		t.Fatalf("recursive join hash differs by input order: %d vs %d", ab.Hash(), ba.Hash())
	}
	again := Types(ab, a, b)
	if !typ.TypeEquals(ab, again) {
		t.Fatalf("recursive join not idempotent: %v vs %v", ab, again)
	}
	if ab.Hash() != again.Hash() {
		t.Fatalf("recursive join hash not idempotent: %d vs %d", ab.Hash(), again.Hash())
	}
}

func TestTypes_RecursiveFamilyJoinPreservesNodeWhenBodyUnchanged(t *testing.T) {
	base := typ.NewRecursive("FlowA", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	withPath := typ.NewRecursive("FlowB", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	joined := Types(base, withPath)
	again := Types(joined, withPath)
	if !typ.SameNode(joined, again) {
		t.Fatalf("idempotent recursive join rebuilt node:\njoined=%v\nagain=%v", joined, again)
	}
}

func TestTypes_DoesNotCoalesceDisjointRecursiveRecordFamilies(t *testing.T) {
	container := typ.NewRecursive("Candidate", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("content", typ.NewRecord().
				Field("parts", typ.NewArray(self)).
				Build()).
			Build()
	})
	leaf := typ.NewRecursive("Part", func(_ typ.Type) typ.Type {
		return typ.NewRecord().
			Field("text", typ.String).
			Build()
	})

	got := Types(container, leaf)
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("Types(disjoint recursive families) = %T %[1]v, want union", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("disjoint recursive union members = %d, want 2: %v", len(union.Members), union)
	}
}

func TestTypes_CoalescesCompatibleRecursiveRecordFields(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	base := typ.NewRecord().
		Field("node", node).
		Field("status", typ.String).
		Build()
	withDetails := typ.NewRecord().
		Field("node", node).
		Field("status", typ.String).
		Field("details", typ.String).
		Build()

	got := Types(base, withDetails)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("Types(recursive field records) = %T %[1]v, want record", got)
	}
	details := rec.GetField("details")
	if details == nil || !details.Optional {
		t.Fatalf("details field = %v, want optional field", details)
	}
}

func TestTypes_BulkCoalescesClosedCompatibleRecordSet(t *testing.T) {
	var members []typ.Type
	for i := 0; i < 64; i++ {
		members = append(members, typ.NewRecord().
			Field("id", typ.String).
			Field("shared", typ.Number).
			Field("field_"+strconv.Itoa(i), typ.Boolean).
			Build())
	}

	got := Types(members...)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("Types(compatible record set) = %T %[1]v, want record", got)
	}
	if rec.GetField("id") == nil || rec.GetField("id").Optional {
		t.Fatalf("common field should remain required: %v", rec.GetField("id"))
	}
	if field := rec.GetField("field_63"); field == nil || !field.Optional {
		t.Fatalf("variant field should become optional: %v", field)
	}
}

func TestTypes_BulkCoalescesClosedCompatibleRecordSubset(t *testing.T) {
	var members []typ.Type
	for i := 0; i < 64; i++ {
		members = append(members, typ.NewRecord().
			Field("id", typ.String).
			Field("field_"+strconv.Itoa(i), typ.Boolean).
			Build())
	}
	members = append(members, typ.String)

	got := Types(members...)
	union, ok := got.(*typ.Union)
	if !ok || len(union.Members) != 2 {
		t.Fatalf("Types(mixed compatible record set) = %T %[1]v, want record|string union", got)
	}
	var rec *typ.Record
	for _, member := range union.Members {
		if r, ok := member.(*typ.Record); ok {
			rec = r
			break
		}
	}
	if rec == nil {
		t.Fatalf("Types(mixed compatible record set) = %v, missing coalesced record member", got)
	}
	if field := rec.GetField("field_63"); field == nil || !field.Optional {
		t.Fatalf("variant field should become optional: %v", field)
	}
}

func TestTypes_BulkCoalescerPreservesDiscriminatedRecordSet(t *testing.T) {
	members := []typ.Type{
		typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("value", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("value", typ.Number).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("extra", typ.Boolean).Build(),
	}

	got := Types(members...)
	u, ok := got.(*typ.Union)
	if !ok || len(u.Members) != 2 {
		t.Fatalf("Types(discriminated record set) = %T %[1]v, want two-variant union", got)
	}
}

func TestTypes_BulkCoalescerPreservesNestedDiscriminatedRecordSet(t *testing.T) {
	chanInt := typ.NewAlias("__test_ChanInt", typ.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	chanStr := typ.NewAlias("__test_ChanStr", typ.NewRecord().
		Field("__tag", typ.LiteralString("str")).
		Build())
	members := []typ.Type{
		typ.NewRecord().
			Field("channel", chanInt).
			Field("value", typ.NewRecord().Field("error", typ.String).Build()).
			Field("ok", typ.Boolean).
			Build(),
		typ.NewRecord().
			Field("channel", chanStr).
			Field("value", typ.NewRecord().Field("data", typ.Number).Build()).
			Field("ok", typ.Boolean).
			Build(),
	}

	got := Types(members...)
	u, ok := got.(*typ.Union)
	if !ok || len(u.Members) != 2 {
		t.Fatalf("Types(nested discriminated record set) = %T %[1]v, want two-variant union", got)
	}
}

func TestTypes_WithNil(t *testing.T) {
	result := Types(typ.String, nil, typ.Number)
	// Nil should be filtered out
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members after filtering nil, got %d", len(u.Members))
	}
}

func TestTypes_AllNil(t *testing.T) {
	result := Types(nil, nil, nil)
	if result != typ.Unknown {
		t.Errorf("all nil should return unknown, got %v", result)
	}
}

func TestTypes_SingleUnknown(t *testing.T) {
	result := Types(typ.Unknown)
	if result != typ.Unknown {
		t.Errorf("single unknown should return unknown, got %v", result)
	}
}

func TestTypes_UnknownAndSingle(t *testing.T) {
	result := Types(typ.Unknown, typ.String)
	if result != typ.String {
		t.Errorf("unknown and string should return string, got %v", result)
	}
}

func TestCoalesceMaps_NilInput(t *testing.T) {
	result := CoalesceMaps(nil)
	if result != nil {
		t.Errorf("nil input should return nil, got %v", result)
	}
}

func TestCoalesceMaps_EmptyInput(t *testing.T) {
	result := CoalesceMaps([]typ.Type{})
	if len(result) != 0 {
		t.Errorf("empty input should return empty, got %d elements", len(result))
	}
}

func TestCoalesceMaps_WithNilElements(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.String, typ.Boolean)
	input := []typ.Type{m1, nil, m2}
	result := CoalesceMaps(input)
	// Should coalesce the two maps and skip nil
	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}
}

func TestCoalesceMaps_ThreeMaps(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.Integer, typ.Boolean)
	m3 := typ.NewMap(typ.Boolean, typ.String)
	input := []typ.Type{m1, m2, m3}
	result := CoalesceMaps(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}
	m, ok := result[0].(*typ.Map)
	if !ok {
		t.Fatalf("expected map, got %T", result[0])
	}
	// Key should be string | integer | boolean
	if _, ok := m.Key.(*typ.Union); !ok {
		t.Errorf("expected union key type, got %T", m.Key)
	}
}

func TestCoalesceEmptyRecordWithMap_MultipleEmptyRecords(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	rec1 := typ.NewRecord().Build()
	rec2 := typ.NewRecord().Build()
	input := []typ.Type{m, rec1, rec2, typ.String}
	result := CoalesceEmptyRecordWithMap(input)
	// Both empty records should be removed
	if len(result) != 2 {
		t.Errorf("expected 2 types (map and string), got %d", len(result))
	}
	for _, r := range result {
		if unwrap.IsEmptyRecord(r) {
			t.Error("empty record should have been removed")
		}
	}
}

func TestCoalesceEmptyRecordWithMap_NilInput(t *testing.T) {
	result := CoalesceEmptyRecordWithMap(nil)
	if result != nil {
		t.Errorf("nil input should return nil, got %v", result)
	}
}

func TestTypes_RecursiveTypes(t *testing.T) {
	// Join recursive types
	rec1 := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})
	rec2 := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})

	// Should deduplicate equivalent recursive types
	result := Types(rec1, rec2)

	// Result should be equivalent to rec1
	if !typ.TypeEquals(result, rec1) {
		t.Error("joining equivalent recursive types should yield single type")
	}
}

func TestTypes_RecursiveWithOther(t *testing.T) {
	rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})

	result := Types(rec, typ.Number)

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(u.Members))
	}
}

func TestTypes_NestedMaps(t *testing.T) {
	// Map<string, Map<string, number>> and Map<string, Map<string, boolean>>
	inner1 := typ.NewMap(typ.String, typ.Number)
	inner2 := typ.NewMap(typ.String, typ.Boolean)
	m1 := typ.NewMap(typ.String, inner1)
	m2 := typ.NewMap(typ.String, inner2)

	input := []typ.Type{m1, m2}
	result := CoalesceMaps(input)

	if len(result) != 1 {
		t.Fatalf("expected 1 coalesced map, got %d", len(result))
	}

	m, ok := result[0].(*typ.Map)
	if !ok {
		t.Fatalf("expected map, got %T", result[0])
	}

	// Value should be coalesced maps
	innerM, ok := m.Value.(*typ.Map)
	if !ok {
		t.Fatalf("expected inner map, got %T", m.Value)
	}

	// Inner value should be number | boolean
	if _, ok := innerM.Value.(*typ.Union); !ok {
		t.Errorf("expected union inner value, got %T", innerM.Value)
	}
}

func TestTypes_OptionalTypes(t *testing.T) {
	opt1 := typ.NewOptional(typ.String)
	opt2 := typ.NewOptional(typ.Number)

	result := Types(opt1, opt2)

	// Should create union of optionals
	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) < 2 {
		t.Errorf("expected at least 2 members, got %d", len(u.Members))
	}
}

func TestTypes_SameTypeMultiple(t *testing.T) {
	// Repeated same type should return single type
	result := Types(typ.Number, typ.Number, typ.Number, typ.Number)
	if result != typ.Number {
		t.Errorf("repeated same type should return single type, got %v", result)
	}
}

func TestTypes_MixedSameAndDifferent(t *testing.T) {
	// Some same, some different
	result := Types(typ.Number, typ.String, typ.Number, typ.String)

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	// Should deduplicate
	if len(u.Members) != 2 {
		t.Errorf("expected 2 unique members, got %d", len(u.Members))
	}
}

func TestFilterUnknown(t *testing.T) {
	tests := []struct {
		name  string
		input []typ.Type
		want  int
	}{
		{"empty", []typ.Type{}, 0},
		{"all unknown", []typ.Type{typ.Unknown, typ.Unknown}, 0},
		{"all nil", []typ.Type{nil, nil}, 0},
		{"mixed", []typ.Type{typ.String, typ.Unknown, typ.Number, nil}, 2},
		{"no unknown", []typ.Type{typ.String, typ.Number}, 2},
	}
	for _, tt := range tests {
		result := filterUnknown(tt.input)
		if len(result) != tt.want {
			t.Errorf("%s: filterUnknown returned %d elements, want %d", tt.name, len(result), tt.want)
		}
	}
}

func TestCoalesceRecordOpenness_NoRecords(t *testing.T) {
	input := []typ.Type{typ.String, typ.Number}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types unchanged, got %d", len(result))
	}
}

func TestCoalesceRecordOpenness_AllOpen(t *testing.T) {
	r1 := typ.NewRecord().SetOpen(true).Field("x", typ.Number).Build()
	r2 := typ.NewRecord().SetOpen(true).Field("y", typ.String).Build()
	input := []typ.Type{r1, r2}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types unchanged, got %d", len(result))
	}
}

func TestCoalesceRecordOpenness_AllClosed(t *testing.T) {
	r1 := typ.NewRecord().Field("x", typ.Number).Build()
	r2 := typ.NewRecord().Field("y", typ.String).Build()
	input := []typ.Type{r1, r2}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Errorf("expected 2 types unchanged, got %d", len(result))
	}
}

func TestCoalesceRecordOpenness_MixedOpenClosed(t *testing.T) {
	open := typ.NewRecord().SetOpen(true).Field("x", typ.Number).Build()
	closed := typ.NewRecord().Field("y", typ.String).Build()
	input := []typ.Type{open, closed}
	result := CoalesceRecordOpenness(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 types, got %d", len(result))
	}
	for _, r := range result {
		rec, ok := r.(*typ.Record)
		if !ok {
			t.Fatalf("expected record, got %T", r)
		}
		if !rec.Open {
			t.Error("all records should be open after coalescing")
		}
	}
}
