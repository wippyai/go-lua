package heap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestHeapSchemaKeepsOneLinkScopedRootAndPayloadAuthority(t *testing.T) {
	linked, schema := heapFixture(t, "heap_structural")
	if schema.KeyCount() == 0 {
		t.Fatal("Heap omitted its sealed root denominator")
	}
	if !schema.ContentID().Available() || schema.LinkContentID() != linked.ContentID() {
		t.Fatal("Heap lost sealed Link identity")
	}
	allocations := 0
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if !ok || key.Kind() != RootAllocation {
			continue
		}
		allocations++
		if _, ok := schema.Reference(key, materialization.Exact); ok {
			t.Fatal("allocation root accepted boot-only Exact role")
		}
		if _, ok := schema.Reference(key, materialization.Recent); !ok {
			t.Fatal("allocation root lost Recent relation")
		}
		if _, ok := schema.Reference(key, materialization.Summary); !ok {
			t.Fatal("allocation root lost Summary relation")
		}
	}
	if allocations == 0 {
		t.Fatal("Heap omitted Program allocation keys")
	}
}

func TestHeapAgeAndCreateRejectKeysOutsideTheirAllocationFence(t *testing.T) {
	linked, schema := heapFixture(t, "heap_age_create_key_fence")
	allocation, _, _ := allocationKeyWithField(t, schema)
	predecessor, predecessorOK := schema.EmptyObject(allocation)
	if !predecessorOK {
		t.Fatal("allocation predecessor")
	}
	if _, ok := schema.Age(predecessor, Key{}); ok {
		t.Fatal("Age accepted a zero key")
	}
	if _, ok := schema.Create(predecessor, Key{}, mutableObject(t, schema)); ok {
		t.Fatal("Create accepted a zero key")
	}

	for index := 0; index < linked.Host().BootRoots().Count(); index++ {
		root, ok := linked.Host().BootRoots().At(index)
		if !ok {
			t.Fatal("boot root")
		}
		boot, ok := schema.KeyForBootRoot(root)
		if !ok {
			t.Fatal("boot key")
		}
		if _, ok := schema.Age(predecessor, boot); ok {
			t.Fatal("Age accepted a boot key")
		}
		if _, ok := schema.Create(predecessor, boot, mutableObject(t, schema)); ok {
			t.Fatal("Create accepted a boot key")
		}
	}

	_, foreign := heapFixture(t, "heap_age_create_foreign")
	foreignKey, _, _ := allocationKeyWithField(t, foreign)
	if _, ok := schema.Age(predecessor, foreignKey); ok {
		t.Fatal("Age accepted a foreign allocation key")
	}
	if _, ok := schema.Create(predecessor, foreignKey, mutableObject(t, schema)); ok {
		t.Fatal("Create accepted a foreign allocation key")
	}
}

func TestHeapSummaryReferenceNeverLicensesExactSelection(t *testing.T) {
	_, schema := heapFixture(t, "heap_summary")
	key, _, _ := allocationKeyWithField(t, schema)
	summary, ok := schema.Reference(key, materialization.Summary)
	if !ok {
		t.Fatal("Summary reference")
	}
	if _, ok := schema.ReferenceSelector(summary); ok {
		t.Fatal("whole-role Summary was treated as an exact key selector")
	}
}

func TestHeapExactNumericKeysReuseOneSlot(t *testing.T) {
	program, err := programlower.Lower(programlower.Source{
		Name: "heap_numeric_exact.lua",
		Text: []byte("return {[1] = 2, [1.0] = 3}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "heap_numeric_exact", Program: program}},
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok {
		t.Fatal("Heap Seal rejected exact numeric fixture")
	}
	for index := 0; index < schema.KeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() != RootAllocation || schema.FieldCount(key) != 2 {
			continue
		}
		left, leftOK := schema.FieldAt(key, 0)
		right, rightOK := schema.FieldAt(key, 1)
		leftSlot, leftSlotOK := schema.SlotForField(left)
		rightSlot, rightSlotOK := schema.SlotForField(right)
		if !leftOK || !rightOK || !leftSlotOK || !rightSlotOK || leftSlot != rightSlot {
			t.Fatal("equal Lua numeric keys did not reuse one Heap slot")
		}
		return
	}
	t.Fatal("exact numeric fixture omitted its two-field allocation")
}

// TestHeapCopiedIndexGeometryRetainsTypedLensAndPayloadRows proves that the
// sealed row is Heap's direct immutable copy: Reads and Writes remain typed,
// exact and dynamic lenses stay distinct, and Values/Position are retained
// only for writes. The first two exact accesses use Lua's equal integer and
// integral-float keys, so both planes must reuse one exact slot.
func TestHeapCopiedIndexGeometryRetainsTypedLensAndPayloadRows(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{
		Name: "heap_geometry_copy.lua",
		Text: []byte(`
local t = {}
local k = 1
local function pair(...) return ... end
local first
first, t[1] = pair()
t[1.0] = pair()
t[k] = pair()
local a = t[1]
local b = t[1.0]
local c = t[k]
return t, a, b, c
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "heap_geometry_copy", Program: p}},
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok {
		t.Fatal("Heap Seal rejected geometry copy fixture")
	}
	if schema.IndexAccessCount() != 6 {
		t.Fatalf("IndexAccess count = %d, want three Reads followed by three Writes", schema.IndexAccessCount())
	}

	var exactReadSlot, dynamicReadSlot Slot
	var exactWriteSlot, dynamicWriteSlot Slot
	readCount, writeCount := 0, 0
	nonzeroWritePosition := false
	for index := 0; index < schema.IndexAccessCount(); index++ {
		access, accessOK := schema.IndexAccessAt(index)
		if !accessOK {
			t.Fatalf("IndexAccessAt(%d)", index)
		}
		geometry, geometryOK := schema.IndexAccessGeometry(access)
		copied, copiedOK := schema.IndexAccessGeometry(access)
		if !geometryOK || !copiedOK || copied != geometry {
			t.Fatalf("IndexGeometry(%d) was not an exact value copy: %#v/%#v/%v/%v", index, geometry, copied, geometryOK, copiedOK)
		}
		if geometry.Shard == (linkproject.Shard{}) || geometry.Base == 0 || geometry.KeyTerm == 0 || geometry.Lens == 0 {
			t.Fatalf("IndexGeometry(%d) omitted direct source terms: %#v", index, geometry)
		}
		slot, slotOK := schema.SlotForIndexAccess(access)
		if !slotOK {
			t.Fatalf("SlotForIndexAccess(%d)", index)
		}
		kind, exact, shard, term, originOK := slot.Origin()
		if !originOK {
			t.Fatalf("slot origin %d", index)
		}
		lensFamily := keyspace.TermFamily(geometry.Lens)
		if geometry.ReadTerm != 0 {
			readCount++
			if geometry.WriteTerm != 0 || geometry.Values != 0 || geometry.Position != -1 || keyspace.TermFamily(geometry.ReadTerm) != keyspace.FamilyRead {
				t.Fatalf("Read row %d lost typed zero/write fields: %#v", index, geometry)
			}
			if _, payloadOK := schema.PayloadForIndexAccess(access); payloadOK {
				t.Fatalf("Read row %d retained a write payload", index)
			}
			if _, resultOK := schema.IndexAccessResult(access); !resultOK {
				t.Fatalf("Read row %d lost its result term", index)
			}
			switch lensFamily {
			case keyspace.FamilyLensExact:
				if kind != SlotExact || shard != (linkproject.Shard{}) || term != 0 {
					t.Fatalf("exact Read %d had dynamic slot origin %v/%v/%v/%v", index, kind, exact, shard, term)
				}
				if readCount == 1 {
					exactReadSlot = slot
				} else if readCount == 2 && slot != exactReadSlot {
					t.Fatal("integer 1 and integral float 1.0 did not reuse one exact Read slot")
				}
			case keyspace.FamilyLensKey:
				if kind != SlotDynamic || exact != (linkproject.Key{}) || shard != geometry.Shard || term != geometry.KeyTerm {
					t.Fatalf("dynamic Read %d lost raw key provenance: %v/%v/%v/%v vs %v/%v", index, kind, exact, shard, term, geometry.Shard, geometry.KeyTerm)
				}
				dynamicReadSlot = slot
			default:
				t.Fatalf("Read %d has non-lens family %v", index, lensFamily)
			}
			continue
		}

		writeCount++
		if geometry.ReadTerm != 0 || keyspace.TermFamily(geometry.WriteTerm) != keyspace.FamilyWrite || geometry.Position < 0 || geometry.Values == 0 {
			t.Fatalf("Write row %d lost typed Values/Position fields: %#v", index, geometry)
		}
		nonzeroWritePosition = nonzeroWritePosition || geometry.Position > 0
		if _, resultOK := schema.IndexAccessResult(access); resultOK {
			t.Fatalf("Write row %d exposed a read result", index)
		}
		payload, payloadOK := schema.PayloadForIndexAccess(access)
		if !payloadOK {
			t.Fatalf("Write row %d lost its payload", index)
		}
		payloadShard, payloadValues, payloadPosition, sourceOK := payload.Source()
		if !sourceOK || payloadShard != geometry.Shard || payloadValues != geometry.Values || payloadPosition != geometry.Position {
			t.Fatalf("Write row %d payload source = %v/%v/%d/%v, want %v/%v/%d/true", index, payloadShard, payloadValues, payloadPosition, sourceOK, geometry.Shard, geometry.Values, geometry.Position)
		}
		switch lensFamily {
		case keyspace.FamilyLensExact:
			if kind != SlotExact || shard != (linkproject.Shard{}) || term != 0 {
				t.Fatalf("exact Write %d had dynamic slot origin %v/%v/%v/%v", index, kind, exact, shard, term)
			}
			if writeCount == 1 {
				exactWriteSlot = slot
			} else if writeCount == 2 && slot != exactWriteSlot {
				t.Fatal("integer 1 and integral float 1.0 did not reuse one exact Write slot")
			}
		case keyspace.FamilyLensKey:
			if kind != SlotDynamic || exact != (linkproject.Key{}) || shard != geometry.Shard || term != geometry.KeyTerm {
				t.Fatalf("dynamic Write %d lost raw key provenance: %v/%v/%v/%v vs %v/%v", index, kind, exact, shard, term, geometry.Shard, geometry.KeyTerm)
			}
			dynamicWriteSlot = slot
		default:
			t.Fatalf("Write %d has non-lens family %v", index, lensFamily)
		}
	}
	if readCount != 3 || writeCount != 3 || exactReadSlot != exactWriteSlot || !nonzeroWritePosition {
		t.Fatalf("typed geometry counts/slot sharing = reads %d writes %d exact %v/%v dynamic %v/%v nonzero %v", readCount, writeCount, exactReadSlot, exactWriteSlot, dynamicReadSlot, dynamicWriteSlot, nonzeroWritePosition)
	}
}

func TestHeapSealAcceptsZeroIndexCandidates(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "heap_zero_geometry.lua", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "heap_zero_geometry", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok || !schema.Valid() || schema.IndexAccessCount() != 0 {
		t.Fatalf("zero-candidate Heap Seal = %v/%v/count %d", ok, schema.Valid(), schema.IndexAccessCount())
	}
}

// The current public lowerer rejects nil table keys before it can publish a
// Flow exact Lens. Keep that source-admission fence explicit: a non-storable
// exact constructor cannot reach Heap as a dynamic or unknown slot, and no
// private Flow mutation is needed to manufacture an invalid candidate.
func TestHeapNonStorableExactConstructorCannotEnterHeap(t *testing.T) {
	for _, source := range []string{`return {[nil] = 1}`, `local t = {}; t[nil] = 1; return t`} {
		if _, err := programlower.Lower(programlower.Source{Name: "heap_non_storable_exact.lua", Text: []byte(source)}); err == nil {
			t.Fatalf("nil exact constructor unexpectedly entered the public Flow seam: %s", source)
		}
	}
}

func TestHeapFreshRuntimeKindsCoverEveryTargetFreshKind(t *testing.T) {
	cases := []struct {
		name  string
		fresh target.FreshKind
		want  runtimekind.Kind
	}{
		{"table", target.FreshTable, runtimekind.Table},
		{"function", target.FreshFunction, runtimekind.Function},
		{"thread", target.FreshThread, runtimekind.Thread},
		{"userdata", target.FreshUserdata, runtimekind.Userdata},
		{"error", target.FreshError, runtimekind.Userdata},
		{"reflection", target.FreshReflection, runtimekind.Userdata},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			schema := sealedFreshHeapFixture(t, test.fresh)
			var fresh Key
			freshCount := 0
			for index := 0; index < schema.KeyCount(); index++ {
				candidate, candidateOK := schema.KeyAt(index)
				if _, _, _, _, _, _, isFresh := candidate.FreshResult(); !candidateOK || !isFresh {
					continue
				}
				fresh = candidate
				freshCount++
			}
			if freshCount != 1 || !fresh.Valid() {
				t.Fatalf("sealed FreshKind(%v) roots=%d/%v, want one", test.fresh, freshCount, fresh.Valid())
			}
			got, ok := schema.owner.rootRuntimeKinds(fresh.slot)
			want := runtimekind.Bit(test.want)
			if !ok || got != want {
				t.Fatalf("sealed FreshKind(%v) runtime mask=%b/%v, want %b/true", test.fresh, got, ok, want)
			}
		})
	}
	if _, ok := freshRootKinds(target.FreshInvalid); ok {
		t.Fatal("invalid FreshKind entered Heap runtime vocabulary")
	}
}

func freshOperation(name string, kind target.FreshKind) target.OperationSpec {
	return target.OperationSpec{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{name}}},
		Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{
			Kind:         flowkind.OutcomeNormal,
			Values:       target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
			FreshResults: []target.FreshResultSpec{{Result: 0, Kind: kind}},
		}},
		Effects: target.RowSpec{Tail: target.RowClosed},
	}
}

// sealedFreshHeapFixture exercises the production Target→Link→Heap path for
// one nominal fresh kind. In particular, Error and Reflection must survive
// sealing as Heap roots rather than being silently discarded by a test-only
// vocabulary mirror.
func sealedFreshHeapFixture(t testing.TB, kind target.FreshKind) Schema {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "heap_fresh_kind.lua", Text: []byte(`return fresh(1)`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{freshOperation("fresh", kind)},
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
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"fresh"}}}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__heap_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__heap_absent"}},
			{Name: "fresh", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "fresh"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "heap_fresh_kind", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok {
		t.Fatalf("Heap Seal rejected FreshKind(%v)", kind)
	}
	return schema
}

func TestHeapFreshRootUsesConservativeFreshKinds(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "heap_fresh_union.lua", Text: []byte(`return selected(1)`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{
			freshOperation("left", target.FreshTable),
			freshOperation("right", target.FreshFunction),
		},
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
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "left"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"left"}}}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "right"}, Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"right"}}}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__heap_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__heap_absent"}},
			{Name: "left", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "left"}},
			{Name: "right", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "right"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "heap_fresh_union", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	want := runtimekind.Bit(runtimekind.Table) | runtimekind.Bit(runtimekind.Function)
	schema, ok := Seal(linked)
	if !ok {
		t.Fatal("Heap Seal rejected fresh root")
	}
	var key Key
	for index := 0; index < schema.KeyCount(); index++ {
		candidate, candidateOK := schema.KeyAt(index)
		if _, _, _, _, _, _, fresh := candidate.FreshResult(); candidateOK && fresh {
			key = candidate
			break
		}
	}
	if !key.Valid() {
		t.Fatal("Heap omitted fresh root")
	}
	if got, ok := schema.owner.rootRuntimeKinds(key.slot); !ok || got != want {
		t.Fatalf("fresh root kinds=%v, want sealed candidate mask=%v", got, want)
	}
}
