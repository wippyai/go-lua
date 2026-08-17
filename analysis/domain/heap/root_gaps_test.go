package heap_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/analysis/domain/type/typecontract"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func portableAnyType() schematype.Type {
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		panic("portable any type")
	}
	return value
}

const compactDynamicHeapSource = `
local key = {}
local child = {}
local record = { [key] = child }
return record, child
`

func compactIndexSpec() *target.Spec {
	return &target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}}
}

func compactDynamicAgeSpec() *target.Spec {
	spec := compactBootSpec()
	spec.Operations = compactIndexSpec().Operations
	spec.InitialEntries = append(spec.InitialEntries, target.InitialEntrySpec{
		Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "seed"},
		Value: target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: 1}, Mutability: target.InitialMutable,
	})
	return spec
}

// TestHeapDynamicAgeFoldsSummaryException exercises the representation that
// matters at the root seam: a dynamic source slot selected by one exact
// Recent reference. Age must move the selected root's Recent containment to
// Summary and fold the now-invalid exact Summary atom into its kind residual.
func TestHeapDynamicAgeFoldsSummaryException(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "compact_age_dynamic", compactDynamicHeapSource, compactDynamicAgeSpec())
	keys := compactAllocationKeys(t, schema, 3)
	selected := Key{}
	for _, key := range keys {
		if schema.FieldCount(key) > 0 {
			selected = key
			break
		}
	}
	if selected == (Key{}) {
		t.Fatal("dynamic Age fixture omitted a field-bearing root")
	}
	other := keys[0]
	if other == selected {
		other = keys[1]
	}
	selectedRecent, recentOK := schema.Reference(selected, materialization.Recent)
	selectedSummary, summaryOK := schema.Reference(selected, materialization.Summary)
	otherRecent, otherOK := schema.Reference(other, materialization.Recent)
	selector, selectorOK := schema.ReferenceSelector(selectedRecent)
	selectedMeta, selectedMetaOK := schema.ContainmentExact(selectedRecent)
	otherContainment, otherContainmentOK := schema.ContainmentExact(otherRecent)
	slot, payload := compactDynamicSlotAndPayload(t, schema)
	state := compactPresent(t, schema, slot, payload, selectedMeta, otherContainment)
	object := compactBuiltObject(t, schema, ShapeEligible, FrozenMutable, selectedMeta, compactObjectStep{selector: selector, state: state})
	input := compactValue(t, schema, selected, object)
	if !recentOK || !summaryOK || !otherOK || !selectorOK || !selectedMetaOK || !otherContainmentOK || !object.Valid() {
		t.Fatal("dynamic Age fixture")
	}

	aged, agedOK := schema.Age(input, selected)
	world, worldOK := aged.WorldAt(0)
	agedObject, objectOK := world.Recent()
	agedMeta, metaOK := agedObject.MetatableAt(0)
	if !agedOK || !worldOK || !objectOK || !metaOK || agedMeta != selectedSummary {
		t.Fatal("Age did not transport the selected Recent metatable")
	}
	if summarySelector, summarySelectorOK := schema.ReferenceSelector(selectedSummary); summarySelectorOK || summarySelector.Valid() {
		t.Fatal("Summary acquired an exact selector")
	}
	seen := false
	if !schema.VisitRawAccess(selected, aged, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		cell, cellOK := access.Cell()
		present, presentOK := cell.PresentAt(0)
		valueChild, keyChild, containmentOK := present.Containment()
		valueReference, valueReferenceOK := valueChild.Reference()
		keyReference, keyReferenceOK := keyChild.Reference()
		if cellOK && presentOK && containmentOK && valueReferenceOK && keyReferenceOK && valueReference == selectedSummary && keyReference == otherRecent {
			seen = true
		}
		return true
	}) || !seen {
		t.Fatal("Age did not fold the invalid Summary atom into the compatible kind residual")
	}

	// The untouched image is the hot path. Keep the allocation law on the
	// exact immutable object rather than measuring fixture construction.
	unchangedObject := compactObject(t, schema, ShapeEligible, FrozenMutable, compactNone(t, schema))
	unchanged := compactValue(t, schema, other, unchangedObject)
	var sink Value
	if allocations := testing.AllocsPerRun(200, func() {
		var ok bool
		sink, ok = schema.Age(unchanged, selected)
		if !ok || !Same(sink, unchanged) {
			t.Fatal("warm unchanged Age image")
		}
	}); allocations != 0 {
		t.Fatalf("warm unchanged Age allocated %v times", allocations)
	}
}

func compactDynamicSlotAndPayload(t testing.TB, schema Schema) (Slot, Payload) {
	t.Helper()
	for index := 0; index < schema.KeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() != RootAllocation {
			continue
		}
		for fieldIndex := 0; fieldIndex < schema.FieldCount(key); fieldIndex++ {
			field, fieldOK := schema.FieldAt(key, fieldIndex)
			slot, slotOK := schema.SlotForField(field)
			payload, payloadOK := schema.PayloadForField(field)
			kind, _, _, originOK := slot.Origin()
			if fieldOK && slotOK && payloadOK && originOK && kind == heapdomain.SlotDynamic {
				return slot, payload
			}
		}
	}
	var dynamic Slot
	for index := 0; index < schema.IndexAccessCount(); index++ {
		access, accessOK := schema.IndexAccessAt(index)
		slot, slotOK := schema.SlotForIndexAccess(access)
		kind, _, _, originOK := slot.Origin()
		if accessOK && slotOK && originOK && kind == heapdomain.SlotDynamic {
			dynamic = slot
			break
		}
	}
	var payload Payload
	for index := 0; index < schema.BootEntryCount() && payload == (Payload{}); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		_, candidate, projectionOK := entry.Projection()
		if entryOK && projectionOK {
			payload = candidate
		}
	}
	for index := 0; index < schema.KeyCount() && payload == (Payload{}); index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() != RootAllocation {
			continue
		}
		for fieldIndex := 0; fieldIndex < schema.FieldCount(key); fieldIndex++ {
			field, fieldOK := schema.FieldAt(key, fieldIndex)
			candidate, payloadOK := schema.PayloadForField(field)
			if fieldOK && payloadOK {
				payload = candidate
				break
			}
		}
	}
	if dynamic == (Slot{}) || payload == (Payload{}) {
		t.Fatal("fixture omitted a dynamic index slot")
	}
	return dynamic, payload
}

func compactFreshOperation(name string, kind target.FreshKind) target.OperationSpec {
	return target.OperationSpec{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{name}}},
		Input:    target.ValuesSpec{Fixed: []schematype.Type{portableAnyType()}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{
			Kind:         flowkind.OutcomeNormal,
			Values:       target.ValuesSpec{Fixed: []schematype.Type{portableAnyType()}, Tail: target.ValuesClosed},
			FreshResults: []target.FreshResultSpec{{Result: 0, Kind: kind}},
		}},
		Effects: target.RowSpec{Tail: target.RowClosed},
	}
}

func compactFreshSpec(operations ...target.OperationSpec) *target.Spec {
	spec := &target.Spec{
		Semantics:  domaincontract.NewSemantics(),
		Operations: operations,
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__heap_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__heap_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__heap_absent"}},
		},
	}
	for _, operation := range operations {
		if len(operation.Bindings) == 0 {
			continue
		}
		binding := operation.Bindings[0]
		if len(binding.Member) == 0 {
			continue
		}
		name := binding.Member[len(binding.Member)-1]
		key := keyspace.LiteralValue{Kind: keyspace.LiteralString, String: name}
		spec.InitialEntries = append(spec.InitialEntries, target.InitialEntrySpec{
			Root: "GlobalEnvRoot", Key: key,
			Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: binding}, Mutability: target.InitialMutable,
		})
		spec.InitialBindings = append(spec.InitialBindings, target.InitialBindingSpec{Name: name, Root: "GlobalEnvRoot", Key: key})
	}
	return spec
}

func compactFreshRoot(t testing.TB, schema Schema) Key {
	t.Helper()
	var fresh Key
	count := 0
	for index := 0; index < schema.KeyCount(); index++ {
		candidate, ok := schema.KeyAt(index)
		_, _, _, _, freshCandidate := candidate.FreshResultID()
		if ok && freshCandidate {
			fresh, count = candidate, count+1
		}
	}
	if count != 1 || !fresh.Valid() {
		t.Fatalf("fresh roots=%d/%v, want one", count, fresh.Valid())
	}
	return fresh
}

// TestHeapArtifactFreshKindRuntimeMaskMatrix keeps fresh-root admission on
// the real Target -> Link -> ProgramArtifact -> Heap seal path. The final
// case uses two operations with one shared (outcome,result,ordinal) template;
// the application must retain the Table|Function may-set on that one root.
func TestHeapArtifactFreshKindRuntimeMaskMatrix(t *testing.T) {
	cases := []struct {
		name string
		kind target.FreshKind
		want runtimekind.Set
	}{
		{"table", target.FreshTable, runtimekind.Bit(runtimekind.Table)},
		{"function", target.FreshFunction, runtimekind.Bit(runtimekind.Function)},
		{"thread", target.FreshThread, runtimekind.Bit(runtimekind.Thread)},
		{"userdata", target.FreshUserdata, runtimekind.Bit(runtimekind.Userdata)},
		{"error", target.FreshError, runtimekind.Bit(runtimekind.Userdata)},
		{"reflection", target.FreshReflection, runtimekind.Bit(runtimekind.Userdata)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, schema, _ := compactHeapFixture(t, "compact_fresh_"+test.name, `return fresh(1)`, compactFreshSpec(compactFreshOperation("fresh", test.kind)))
			root := compactFreshRoot(t, schema)
			reference, referenceOK := schema.Reference(root, materialization.Recent)
			selector, selectorOK := schema.ReferenceSelector(reference)
			if !referenceOK || !selectorOK || selector.RuntimeKinds() != test.want {
				t.Fatalf("FreshKind(%v) runtime mask=%b/%v, want %b", test.kind, selector.RuntimeKinds(), selectorOK, test.want)
			}
		})
	}

	union := compactFreshSpec(compactFreshOperation("left", target.FreshTable), compactFreshOperation("right", target.FreshFunction))
	_, unionSchema, _ := compactHeapFixture(t, "compact_fresh_union", `return selected(1)`, union)
	root := compactFreshRoot(t, unionSchema)
	reference, referenceOK := unionSchema.Reference(root, materialization.Recent)
	selector, selectorOK := unionSchema.ReferenceSelector(reference)
	want := runtimekind.Bit(runtimekind.Table) | runtimekind.Bit(runtimekind.Function)
	if !referenceOK || !selectorOK || selector.RuntimeKinds() != want {
		t.Fatalf("shared fresh template runtime mask=%b/%v, want %b", selector.RuntimeKinds(), selectorOK, want)
	}
}

// TestHeapPhysicalIndexSlotAndProvenanceMatrix keeps Lua's numeric equality
// quotient and the direct typed geometry rows together. Exact and dynamic
// source provenance must agree in both read and write planes; a literal-only
// program must not manufacture an index occurrence.
func TestHeapPhysicalIndexSlotAndProvenanceMatrix(t *testing.T) {
	source := `
local t = {}
local k = 1
t[1] = 2
t[1.0] = 3
t[k] = 4
local a = t[1]
local b = t[1.0]
local c = t[k]
return t, a, b, c
`
	_, schema, _ := compactHeapFixture(t, "compact_physical_index", source, compactIndexSpec())
	if schema.IndexAccessCount() != 6 {
		t.Fatalf("index access count=%d, want six typed rows", schema.IndexAccessCount())
	}
	var exactRead, exactWrite Slot
	readCount, writeCount := 0, 0
	nonzeroWritePosition := false
	for index := 0; index < schema.IndexAccessCount(); index++ {
		access, accessOK := schema.IndexAccessAt(index)
		geometry, geometryOK := schema.IndexAccessGeometry(access)
		slot, slotOK := schema.SlotForIndexAccess(access)
		kind, exact, keyValue, originOK := slot.Origin()
		if !accessOK || !geometryOK || !slotOK || !originOK || !geometry.Module.Available() || !geometry.ProgramID.Available() {
			t.Fatalf("index row %d lost sealed geometry/provenance", index)
		}
		if geometry.Read {
			readCount++
			if geometry.Position != -1 || geometry.ValuesID.Available() {
				t.Fatalf("read row %d retained write coordinates: %#v", index, geometry)
			}
			if resultID, resultOK := schema.IndexAccessResultID(access); !resultOK || !resultID.Available() {
				t.Fatalf("read row %d lost detached result identity", index)
			}
			if _, payloadOK := schema.PayloadForIndexAccess(access); payloadOK {
				t.Fatalf("read row %d retained a payload", index)
			}
			if kind == heapdomain.SlotExact {
				literal, literalOK := exact.Literal()
				if !literalOK || literal.Kind != keyspace.LiteralInteger || literal.Integer != 1 || geometry.DynamicKey || keyValue.Available() {
					t.Fatalf("exact read %d provenance=%v/%#v/%v", index, kind, literal, keyValue)
				}
				if exactRead != (Slot{}) && exactRead != slot {
					t.Fatal("integer 1 and integral float 1.0 split read slots")
				}
				exactRead = slot
			} else if kind != heapdomain.SlotDynamic || !geometry.DynamicKey || !keyValue.Available() || keyValue != geometry.KeyValueID {
				t.Fatalf("dynamic read %d provenance=%v/%v/%v", index, kind, geometry.DynamicKey, keyValue)
			}
			continue
		}

		writeCount++
		if geometry.Position > 0 {
			nonzeroWritePosition = true
		}
		if geometry.Position < 0 || !geometry.ValuesID.Available() || geometry.DynamicKey && !keyValue.Available() {
			t.Fatalf("write row %d lost position/source coordinates: %#v", index, geometry)
		}
		if _, resultOK := schema.IndexAccessResultID(access); resultOK {
			t.Fatalf("write row %d unexpectedly retained detached result identity", index)
		}
		payload, payloadOK := schema.PayloadForIndexAccess(access)
		payloadModule, payloadValues, payloadPosition, sourceOK := payload.Source()
		if !payloadOK || !sourceOK || payloadModule != geometry.Module || payloadValues != geometry.ValuesID || payloadPosition != geometry.Position {
			t.Fatalf("write row %d payload provenance", index)
		}
		if kind == heapdomain.SlotExact {
			literal, literalOK := exact.Literal()
			if !literalOK || literal.Kind != keyspace.LiteralInteger || literal.Integer != 1 || geometry.DynamicKey || keyValue.Available() {
				t.Fatalf("exact write %d provenance=%v/%#v/%v", index, kind, literal, keyValue)
			}
			if exactWrite != (Slot{}) && exactWrite != slot {
				t.Fatal("integer 1 and integral float 1.0 split write slots")
			}
			exactWrite = slot
		} else if kind != heapdomain.SlotDynamic || !geometry.DynamicKey || !keyValue.Available() || keyValue != geometry.KeyValueID {
			t.Fatalf("dynamic write %d provenance=%v/%v/%v", index, kind, geometry.DynamicKey, keyValue)
		}
	}
	if readCount != 3 || writeCount != 3 || !nonzeroWritePosition || exactRead == (Slot{}) || exactRead != exactWrite {
		t.Fatalf("typed index counts/read-write slot sharing=%d/%d/%v/%v", readCount, writeCount, exactRead, exactWrite)
	}

	_, zeroSchema, _ := compactHeapFixture(t, "compact_physical_zero", `return 1`, nil)
	if zeroSchema.IndexAccessCount() != 0 {
		t.Fatalf("zero-index fixture count=%d, want zero", zeroSchema.IndexAccessCount())
	}
}
