package source_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/internal/testfixture"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	source "github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestRootClassifiesCompleteSourceConstructorForms(t *testing.T) {
	heap, values, _ := sourceFixture(t, `
local zero = {}
local closure = function() end
local fixed = { answer = 42 }
local f = function() return 1 end
local finalopen = { f() }
return zero, closure, fixed, finalopen
`)
	seen := map[source.Form]int{}
	for _, allocation := range allocationKeys(heap) {
		root, rootOK := source.New(heap, allocation)
		if !rootOK || !root.Revalidate(heap) {
			t.Fatal("source root classification")
		}
		seen[root.Form()]++
		if root.Form() == source.FormClosed {
			closed, closedOK := source.NewClosed(heap, values, allocation)
			if !closedOK || !closed.Revalidate() || closed.Count() != 1 || closed.CoordinateCount() != 1 {
				t.Fatal("closed descriptor")
			}
			field, fieldOK := closed.At(0)
			if !fieldOK || field.Ordinal() != 1 || field.ValueOrdinal() != 0 {
				t.Fatal("closed field ordinal")
			}
			if _, selectorOK := field.ExactSelector(); !selectorOK {
				t.Fatal("static field omitted exact selector")
			}
		}
	}
	if seen[source.FormEmpty] < 2 || seen[source.FormClosed] != 1 || seen[source.FormFinalOpen] != 1 {
		t.Fatalf("forms=%v", seen)
	}
}

// TestRootFenceIsConstructorReceiptOnly proves the hot Root fence authenticates
// a constructor-issued scalar receipt without re-entering Link/Flow source
// classification. Full classification remains covered by New/Revalidate.
func TestRootFenceIsConstructorReceiptOnly(t *testing.T) {
	heap, _, linked := sourceFixture(t, `local x = {}; return x`)
	keys := allocationKeys(heap)
	if len(keys) == 0 {
		t.Fatal("allocation root")
	}
	root, rootOK := source.New(heap, keys[0])
	if !rootOK || !root.FencedTo(heap) {
		t.Fatal("constructor root fence")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if !root.FencedTo(heap) {
			panic("receipt fence")
		}
	}); allocations != 0 {
		t.Fatalf("hot Root fence allocated %f", allocations)
	}
	var forged source.Root
	if forged.FencedTo(heap) {
		t.Fatal("root without constructor receipt passed hot fence")
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("source compilation")
	}
	foreign, foreignFailure := heapdomain.SealWithArtifacts(linked, sourceHeapMounts(t, linked, compilation))
	if foreignFailure != heapdomain.SealFailureNone || foreign == heap || root.FencedTo(foreign) {
		t.Fatal("equal-content foreign Heap schema crossed Root receipt")
	}
}

func TestClosedDenseCoordinatesFenceDynamicAndFinalOpenSources(t *testing.T) {
	heap, values, _ := sourceFixture(t, `
local x = {}
local y = function() end
local repeated = { [x] = x, [x] = x }
local sparse = { [y] = x }
local open = { x, y() }
return repeated, sparse, open
`)
	var repeated, sparse heapdomain.Key
	for _, allocation := range allocationKeys(heap) {
		root, rootOK := source.New(heap, allocation)
		if !rootOK {
			t.Fatal("source root")
		}
		switch {
		case root.Form() == source.FormClosed && heap.FieldCount(allocation) == 2:
			repeated = allocation
		case root.Form() == source.FormClosed && heap.FieldCount(allocation) == 1:
			sparse = allocation
		case root.Form() == source.FormFinalOpen:
			if _, ok := source.NewClosed(heap, values, allocation); ok {
				t.Fatal("final-open table entered scalar closed descriptor")
			}
		}
	}
	repeatedClosed, repeatedOK := source.NewClosed(heap, values, repeated)
	sparseClosed, sparseOK := source.NewClosed(heap, values, sparse)
	if !repeatedOK || !sparseOK || repeatedClosed.Count() != 2 || repeatedClosed.CoordinateCount() != 2 || sparseClosed.CoordinateCount() != 2 {
		t.Fatalf("closed coordinate denominator repeated=%t/%d/%d sparse=%t/%d/%d", repeatedOK, repeatedClosed.Count(), repeatedClosed.CoordinateCount(), sparseOK, sparseClosed.Count(), sparseClosed.CoordinateCount())
	}
	first, firstOK := repeatedClosed.At(0)
	second, secondOK := repeatedClosed.At(1)
	if !firstOK || !secondOK || first.Ordinal() != 1 || second.Ordinal() != 2 || first.KeyKind() != source.KeyDynamic || second.KeyKind() != source.KeyDynamic || first.ValueOrdinal() >= uint32(repeatedClosed.CoordinateCount()) || second.ValueOrdinal() >= uint32(repeatedClosed.CoordinateCount()) {
		t.Fatal("repeated dynamic source order")
	}
	firstKey, firstKeyOK := first.DynamicKeyOrdinal()
	secondKey, secondKeyOK := second.DynamicKeyOrdinal()
	if !firstKeyOK || !secondKeyOK || firstKey != first.ValueOrdinal() || secondKey != second.ValueOrdinal() || firstKey >= uint32(repeatedClosed.CoordinateCount()) || secondKey >= uint32(repeatedClosed.CoordinateCount()) {
		t.Fatal("dynamic key use escaped dense coordinate vector")
	}
	firstKeyCoordinate, firstKeyCoordinateOK := first.DynamicKey()
	secondKeyCoordinate, secondKeyCoordinateOK := second.DynamicKey()
	if !firstKeyCoordinateOK || !secondKeyCoordinateOK || first.Value() != firstKeyCoordinate || second.Value() != secondKeyCoordinate || first.Value() == second.Value() {
		t.Fatal("direct same-cell fields did not retain one local occurrence each")
	}
	left, leftOK := sparseClosed.At(0)
	if !leftOK || left.ValueOrdinal() > 1 {
		t.Fatal("sparse source did not use a dense value ordinal")
	}
	key, keyOK := left.DynamicKeyOrdinal()
	if !keyOK || key > 1 || key == left.ValueOrdinal() {
		t.Fatal("two source coordinates did not retain two dense local ordinals")
	}
	foreignHeap, foreignValues, _ := sourceFixture(t, `return {}`)
	if foreignHeap.ContentID() == heap.ContentID() || foreignValues == values {
		t.Fatal("foreign fixture")
	}
	if _, ok := source.NewClosed(foreignHeap, values, repeated); ok {
		t.Fatal("foreign Heap admitted source descriptor")
	}
	if _, ok := source.NewClosed(heap, foreignValues, repeated); ok {
		t.Fatal("foreign Value schema admitted source descriptor")
	}
}

func TestClosedRevalidateForFencesExactSchemaInstances(t *testing.T) {
	heap, values, linked := sourceFixture(t, `local x = {}; return { [x] = x }`)
	var allocation heapdomain.Key
	for _, candidate := range allocationKeys(heap) {
		if heap.FieldCount(candidate) == 1 {
			allocation = candidate
			break
		}
	}
	closed, closedOK := source.NewClosed(heap, values, allocation)
	root, rootOK := source.New(heap, allocation)
	if !closedOK || !rootOK || !root.FencedTo(heap) || !closed.RevalidateFor(heap, values) {
		t.Fatal("exact source schemas did not revalidate")
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("source compilation")
	}
	otherHeap, otherHeapFailure := heapdomain.SealWithArtifacts(linked, sourceHeapMounts(t, linked, compilation))
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	otherValues, otherValuesFailure := valuedomain.SealWithFailure(linked, otherHeap, calltest.MustSeal(t, linked, sourceValueMounts(t, linked, compilation)), sourceValueMounts(t, linked, compilation), structural)
	var otherAllocation heapdomain.Key
	for index := 0; index < otherHeap.KeyCount(); index++ {
		candidate, candidateOK := otherHeap.KeyAt(index)
		candidateID, candidateIDOK := otherHeap.KeyID(candidate)
		allocationID, allocationIDOK := heap.KeyID(allocation)
		if candidateOK && candidateIDOK && allocationIDOK && candidateID == allocationID {
			otherAllocation = candidate
			break
		}
	}
	localReplay, localReplayOK := source.NewClosed(otherHeap, otherValues, otherAllocation)
	if otherHeapFailure != heapdomain.SealFailureNone || otherValuesFailure != valuedomain.SealFailureNone || !otherAllocation.Valid() || !localReplayOK || !localReplay.Revalidate() || root.FencedTo(otherHeap) || closed.RevalidateFor(otherHeap, values) || closed.RevalidateFor(heap, otherValues) || closed.RevalidateFor(otherHeap, otherValues) {
		t.Fatal("independently sealed same-Link schemas crossed source fence")
	}
	if _, mixedHeapOK := source.NewClosed(otherHeap, values, otherAllocation); mixedHeapOK {
		t.Fatal("same-content foreign Value schema crossed Heap owner fence")
	}
	if _, mixedValueOK := source.NewClosed(heap, otherValues, allocation); mixedValueOK {
		t.Fatal("same-content foreign Heap schema crossed Value owner fence")
	}
}

func TestClosedEffectBetweenSameCellUsesDoesNotShareCoordinates(t *testing.T) {
	heap, values, _ := sourceFixture(t, `
local x = {}
local function mutate()
  x = {}
  return x
end
local direct = { [x] = x }
local changed = { [x] = mutate() }
return direct, changed
`)
	var direct, changed source.Closed
	for _, allocation := range allocationKeys(heap) {
		if heap.FieldCount(allocation) != 1 {
			continue
		}
		closed, closedOK := source.NewClosed(heap, values, allocation)
		if !closedOK {
			t.Fatal("closed one-field allocation")
		}
		if closed.CoordinateCount() == 1 {
			direct = closed
		} else if closed.CoordinateCount() == 2 {
			changed = closed
		}
	}
	if direct.Count() != 1 || changed.Count() != 1 || direct.CoordinateCount() != 1 || changed.CoordinateCount() != 2 {
		t.Fatal("effectful field did not retain separate coordinates")
	}
	directField, directOK := direct.At(0)
	changedField, changedOK := changed.At(0)
	directKey, directKeyOK := directField.DynamicKeyOrdinal()
	changedKey, changedKeyOK := changedField.DynamicKeyOrdinal()
	if !directOK || !changedOK || !directKeyOK || !changedKeyOK || directField.ValueOrdinal() != directKey || changedField.ValueOrdinal() == changedKey {
		t.Fatal("same-cell mutation crossed direct-read coordinate proof")
	}
}

func TestClosedAllocationSourceKindsUseAuthoredFieldGeometry(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		literal    keyspace.LiteralValue
		fieldKind  source.KeyKind
		wantString bool
	}{
		{
			name:       "name",
			text:       `local child = {}; return { answer = child }`,
			literal:    keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "answer"},
			fieldKind:  source.KeyExact,
			wantString: true,
		},
		{
			name:      "list",
			text:      `local child = {}; return { child }`,
			literal:   keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1},
			fieldKind: source.KeyExact,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, _ := oneClosedField(t, test.text)
			if field.KeyKind() != test.fieldKind {
				t.Fatalf("field key kind = %v, want %v", field.KeyKind(), test.fieldKind)
			}
			exact, exactOK := field.ExactKey()
			literal, literalOK := exact.Literal()
			if !exactOK || !literalOK || literal != test.literal {
				t.Fatalf("field exact key = %v/%v, Link literal = %#v/%v, want %#v", exact, exactOK, literal, literalOK, test.literal)
			}
			if test.wantString && literal.Kind != keyspace.LiteralString {
				t.Fatal("name field did not retain a string exact key")
			}
		})
	}
}

func TestClosedAllocationIntegralFloatAndIntegerKeysShareCanonicalLinkKey(t *testing.T) {
	fields, _ := closedFieldsFixture(t, `local child = {}; return { [1] = child, [1.0] = child }`)
	if len(fields) != 2 {
		t.Fatalf("closed fields = %d, want two exact fields", len(fields))
	}
	left, leftOK := fields[0].ExactKey()
	right, rightOK := fields[1].ExactKey()
	if !leftOK || !rightOK || left != right {
		t.Fatalf("canonical exact keys = %v/%v and %v/%v, want one Link key", left, leftOK, right, rightOK)
	}
	literal, literalOK := left.Literal()
	if !literalOK || literal != (keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}) {
		t.Fatalf("canonical Link literal = %#v/%v, want integer 1", literal, literalOK)
	}
}

func TestClosedAllocationFieldKeyRemainsDynamicAndUsesSameCellOptimization(t *testing.T) {
	// Lowering mints separate Read occurrences for the key and value; the
	// source Cell proof, rather than occurrence-term equality, permits one
	// coordinate here.
	field, _ := oneClosedField(t, `local key = {}; return { [key] = key }`)
	if field.KeyKind() != source.KeyDynamic {
		t.Fatalf("field key kind = %v, want dynamic", field.KeyKind())
	}
	if _, exact := field.ExactKey(); exact {
		t.Fatal("dynamic FieldKey retained an exact key")
	}
	keyOrdinal, dynamicOK := field.DynamicKeyOrdinal()
	if !dynamicOK || keyOrdinal != field.ValueOrdinal() {
		t.Fatalf("same-cell dynamic ordinals = %v/%v, value ordinal = %v", keyOrdinal, dynamicOK, field.ValueOrdinal())
	}
}

func TestClosedDynamicKeyDoesNotCoalesceIndependentOrLensReads(t *testing.T) {
	heap, values, _ := sourceFixture(t, `
local key = {}
local value = {}
local base = {}
local direct = { [key] = key }
local independent = { [key] = value }
local lens = { [base.name] = base.name }
return direct, independent, lens
`)
	counts := map[int]int{}
	closedCount := 0
	for _, allocation := range allocationKeys(heap) {
		if heap.FieldCount(allocation) != 1 {
			continue
		}
		closed, closedOK := source.NewClosed(heap, values, allocation)
		if !closedOK {
			t.Fatal("closed dynamic-read allocation")
		}
		closedCount++
		counts[closed.CoordinateCount()]++
	}
	if closedCount != 3 || counts[1] != 1 || counts[2] != 2 {
		t.Fatalf("direct/independent/lens coordinate counts = %v across %d tables, want one 1 and two 2", counts, closedCount)
	}
}

// allocationKeys is the test-side admission boundary for Program allocations.
// It deliberately enumerates Heap's issued coordinates rather than rebuilding
// Link's retired allocation relation.
// TestClosedSummaryVectorSpellsAbsenceAsANegativeCount states the summary-key
// seam Closed shares with the engine: a count is the length of an
// authenticated key vector, and the absence of any vector is a negative
// count. Zero is a length, so an operand that never passed its constructor
// fence must not report one; an authenticated closed operand always carries
// one key per dense coordinate.
func TestClosedSummaryVectorSpellsAbsenceAsANegativeCount(t *testing.T) {
	var unauthenticated source.Closed
	if count := unauthenticated.SummaryKeyCount(); count >= 0 {
		t.Fatalf("unauthenticated closed operand reported a %d-key vector, want a negative absence", count)
	}
	if _, ok := unauthenticated.SummaryKeyAt(0); ok {
		t.Fatal("unauthenticated closed operand served a summary key")
	}
	heap, values, _ := sourceFixture(t, `return { answer = 42, other = 7 }`)
	seen := 0
	for _, allocation := range allocationKeys(heap) {
		closed, closedOK := source.NewClosed(heap, values, allocation)
		if !closedOK {
			continue
		}
		seen++
		if closed.SummaryKeyCount() != closed.CoordinateCount() || closed.SummaryKeyCount() < 1 {
			t.Fatalf("authenticated closed operand reported %d keys for %d coordinates", closed.SummaryKeyCount(), closed.CoordinateCount())
		}
		if _, ok := closed.SummaryKeyAt(closed.SummaryKeyCount()); ok {
			t.Fatal("closed operand served a key past its vector")
		}
	}
	if seen == 0 {
		t.Fatal("closed summary-vector fixture")
	}
}

func allocationKeys(schema heapdomain.Schema) []heapdomain.Key {
	keys := make([]heapdomain.Key, 0, schema.KeyCount())
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if ok && key.Kind() == heapdomain.RootAllocation {
			keys = append(keys, key)
		}
	}
	return keys
}

func oneClosedField(t testing.TB, text string) (source.Field, *link.Link) {
	t.Helper()
	fields, linked := closedFieldsFixture(t, text)
	if len(fields) != 1 {
		t.Fatalf("closed fields = %d, want one", len(fields))
	}
	return fields[0], linked
}

func closedFieldsFixture(t testing.TB, text string) ([]source.Field, *link.Link) {
	t.Helper()
	heap, values, linked := sourceFixture(t, text)
	fields := make([]source.Field, 0, 2)
	for _, allocation := range allocationKeys(heap) {
		closed, closedOK := source.NewClosed(heap, values, allocation)
		if !closedOK {
			continue
		}
		for index := 0; index < closed.Count(); index++ {
			field, fieldOK := closed.At(index)
			if !fieldOK {
				t.Fatalf("closed field[%d] is unavailable", index)
			}
			fields = append(fields, field)
		}
	}
	return fields, linked
}

func TestCoordinateOrdinalDeduplicatesRepeatedUses(t *testing.T) {
	_, values, linked := sourceFixture(t, `local x = {}; return x`)
	value, valueOK := linked.Boundary().Values().At(0)
	valueID, valueIDOK := linked.Boundary().Values().ID(value)
	coordinate, coordinateOK := values.CoordinateForID(valueID)
	if !valueOK || !valueIDOK || !coordinateOK {
		t.Fatal("coordinate")
	}
	if ordinal, ok := sourceCoordinateOrdinal([]valuedomain.Coordinate{coordinate}, coordinate); !ok || ordinal != 0 {
		t.Fatal("repeated coordinate did not map to one dense ordinal")
	}
}

func sourceCoordinateOrdinal(coordinates []valuedomain.Coordinate, want valuedomain.Coordinate) (uint32, bool) {
	for index, coordinate := range coordinates {
		if coordinate == want {
			return uint32(index), true
		}
	}
	return 0, false
}

func sourceFixture(t testing.TB, text string) (heapdomain.Schema, *valuedomain.Schema, *link.Link) {
	t.Helper()
	p, err := lualower.Lower(lualower.Source{Name: "allocation_source.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	requireOperation, requireErr := testfixture.ScopedRequireOperation()
	if requireErr != nil {
		t.Fatal(requireErr)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{requireOperation}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "allocation_source", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("source compilation")
	}
	heapMounts := sourceHeapMounts(t, linked, compilation)
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, heapMounts)
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, calltest.MustSeal(t, linked, sourceValueMounts(t, linked, compilation)), sourceValueMounts(t, linked, compilation), structural)
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatal("Heap schema")
	}
	if valueFailure != valuedomain.SealFailureNone {
		t.Fatal("Value schema")
	}
	return heap, values, linked
}

func sourceHeapMounts(t testing.TB, linked *link.Link, compilation composite.Compilation) []programmount.MountedArtifact {
	t.Helper()
	heapMounts, _ := sourceMountedArtifacts(t, linked, compilation)
	return heapMounts
}

func sourceValueMounts(t testing.TB, linked *link.Link, compilation composite.Compilation) []programmount.MountedArtifact {
	t.Helper()
	_, valueMounts := sourceMountedArtifacts(t, linked, compilation)
	return valueMounts
}

func sourceMountedArtifacts(t testing.TB, linked *link.Link, compilation composite.Compilation) ([]programmount.MountedArtifact, []programmount.MountedArtifact) {
	t.Helper()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilation.Available() || !executionSchemaID.Available() || !issuanceOK || linked == nil || linked.Project() == nil {
		t.Fatal("source artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	heapMounts := make([]programmount.MountedArtifact, projectMounts.Count())
	valueMounts := make([]programmount.MountedArtifact, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		_, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("source artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("source artifact: %v", failure)
		}
		var heapOK, valueOK bool
		heapMounts[index], heapOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		valueMounts[index], valueOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		if !heapOK || !valueOK {
			t.Fatal("source artifact mount receipt")
		}
	}
	return heapMounts, valueMounts
}

// TestClosedOperandVectorIsTheOneValueAxisPublishes is the single-authority
// law. WHICH Value coordinates a constructor reads, and in which order, is a
// fact in Value's own numbering: a coordinate's dense index is the position
// that axis assigned it, and no upstream owner holds one. So the axis states
// the vector and this descriptor composes its field topology over it.
//
// The law is that the two are the SAME vector, coordinate for coordinate and
// key for key, for every closed constructor in a real program - not that they
// agree in width, which two independent walks would also manage while
// disagreeing about order. A rule reading these operands is spanned by the
// published vector while the fold applies values through these fields, so a
// drift between them would pair every cell with the wrong field.
func TestClosedOperandVectorIsTheOneValueAxisPublishes(t *testing.T) {
	heaps, values, _ := sourceFixture(t, `
local x = {}
local y = function() end
local repeated = { [x] = x, [x] = x }
local sparse = { [y] = x }
local named = { alpha = x, beta = y }
return repeated, sparse, named
`)
	constructors := 0
	for _, allocation := range allocationKeys(heaps) {
		closed, closedOK := source.NewClosed(heaps, values, allocation)
		if !closedOK {
			continue
		}
		constructors++
		published, publishedOK := values.ClosedOperandCoordinates(allocation)
		if !publishedOK {
			t.Fatalf("value publishes no operand vector for a constructor it admitted: %v", allocation)
		}
		if closed.CoordinateCount() != len(published) || closed.SummaryKeyCount() != len(published) {
			t.Fatalf("operand vector widths disagree: descriptor=%d keys=%d published=%d",
				closed.CoordinateCount(), closed.SummaryKeyCount(), len(published))
		}
		for index := range published {
			coordinate, coordinateOK := closed.CoordinateAt(index)
			key, keyOK := closed.SummaryKeyAt(index)
			dense, denseOK := values.ClosedOperandKeyAt(allocation, index)
			if !coordinateOK || !keyOK || !denseOK {
				t.Fatalf("operand %d of %v is unreadable", index, allocation)
			}
			if coordinate != published[index] {
				t.Fatalf("operand %d of %v is a different coordinate than the one Value published", index, allocation)
			}
			if key != uint64(dense) {
				t.Fatalf("operand %d of %v is delivered at key %d, published at %d", index, allocation, key, dense)
			}
		}
		for index := 0; index < closed.Count(); index++ {
			field, fieldOK := closed.At(index)
			if !fieldOK {
				t.Fatalf("field %d of %v is unreadable", index, allocation)
			}
			ordinal := field.ValueOrdinal()
			if int(ordinal) >= len(published) || published[ordinal] != field.Value() {
				t.Fatalf("field %d of %v addresses position %d of the published vector, which holds another coordinate", index, allocation, ordinal)
			}
		}
	}
	if constructors == 0 {
		t.Fatal("the fixture admitted no closed constructor, so the law proved nothing")
	}
}
