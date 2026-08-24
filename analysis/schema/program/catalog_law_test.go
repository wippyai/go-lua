package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func catalogLawSchema(t *testing.T) identity.ContentID {
	t.Helper()
	id, derived := identity.DeriveContentID("cold-law/runtime-schema", nil)
	if !derived {
		t.Fatal("runtime schema identity")
	}
	return id
}

// The cold catalog and the runtime schema it is derived from are two
// identities. That is what keeps a cold axis from being a structurally valid
// address into a runtime publication's slot of the same number.
func TestCatalogIsDistinctFromItsRuntimeSchema(t *testing.T) {
	runtime := catalogLawSchema(t)
	catalog, derived := programcatalog.CatalogID(runtime)
	if !derived || !catalog.Available() {
		t.Fatal("cold catalog derived nothing")
	}
	if catalog == runtime {
		t.Fatal("the cold catalog is the runtime schema, so a cold axis addresses a runtime column")
	}

	other, otherDerived := identity.DeriveContentID("cold-law/other-runtime-schema", nil)
	if !otherDerived {
		t.Fatal("second runtime schema")
	}
	second, secondDerived := programcatalog.CatalogID(other)
	if !secondDerived || second == catalog {
		t.Fatal("two declaration catalogs derived one cold catalog")
	}
	if _, derived := programcatalog.CatalogID(identity.ContentID{}); derived {
		t.Fatal("an unavailable runtime schema derived a cold catalog")
	}
}

func catalogLawFamily() []calltarget.Target {
	first, firstOK := calltarget.NewTarget(identity.ContentID{1}, identity.ContentID{2}, identity.ContentID{3}, identity.ContentID{4}, identity.ContentID{5})
	second, secondOK := calltarget.NewTarget(identity.ContentID{6}, identity.ContentID{7}, identity.ContentID{8}, identity.ContentID{9}, identity.ContentID{10})
	if !firstOK || !secondOK {
		panic("catalog law target")
	}
	return []calltarget.Target{first, second}
}

func sealCatalogLaw(t *testing.T, rows []calltarget.Target) (snapshot.Frozen, identity.ContentID) {
	t.Helper()
	catalog, derived := programcatalog.CatalogID(catalogLawSchema(t))
	if !derived {
		t.Fatal("cold catalog")
	}
	content, sealed := calltarget.Family().Content(rows, catalog)
	if !sealed {
		t.Fatal("call target content")
	}
	builder := snapshot.NewFrozen(catalog, identity.StoreID(7))
	if err := snapshot.PutFrozenColumn(&builder, calltarget.Family().Axis(catalog), content); err != nil {
		t.Fatalf("put cold column: %v", err)
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return frozen, catalog
}

// A cold family is read back in the order it was emitted, and its width is the
// sealed universe rather than a count a reader has to be told separately.
func TestCallTargetFamilyReadsBackInEmittedOrder(t *testing.T) {
	rows := catalogLawFamily()
	frozen, catalog := sealCatalogLaw(t, rows)

	count, published := calltarget.Family().Count(&frozen, catalog)
	if !published || count != len(rows) {
		t.Fatalf("family width = %d (published %t), want %d", count, published, len(rows))
	}
	for index, want := range rows {
		got, held := calltarget.Family().At(&frozen, catalog, index)
		if !held || got != want {
			t.Fatalf("ordinal %d read %+v, emitted %+v", index, got, want)
		}
	}
	if _, held := calltarget.Family().At(&frozen, catalog, len(rows)); held {
		t.Fatal("an ordinal past the family read a row")
	}
	if _, held := calltarget.Family().At(&frozen, catalog, -1); held {
		t.Fatal("a negative ordinal read a row")
	}
}

// An ordinal past the sealed family is a proven absence, not ignorance: the
// column publishes the family's own ordinal range as its key universe.
func TestCallTargetFamilyProvesItsOwnBound(t *testing.T) {
	rows := catalogLawFamily()
	frozen, catalog := sealCatalogLaw(t, rows)

	if _, status := snapshot.ReadFrozen(&frozen, calltarget.Family().Axis(catalog), programfamily.Ordinal(len(rows)-1)); status != snapshot.ReadHit {
		t.Fatalf("last ordinal reported %v", status)
	}
	if _, status := snapshot.ReadFrozen(&frozen, calltarget.Family().Axis(catalog), programfamily.Ordinal(len(rows))); status == snapshot.ReadHit {
		t.Fatal("an ordinal past the family reported a hit")
	}
}

// A family the compiler could not prove seals nothing: a program either
// proved every target it emitted or it did not compile.
func TestCallTargetContentRejectsAnUnprovenRow(t *testing.T) {
	_, derived := programcatalog.CatalogID(catalogLawSchema(t))
	if !derived {
		t.Fatal("cold catalog")
	}
	if _, complete := calltarget.NewTarget(identity.ContentID{11}, identity.ContentID{}, identity.ContentID{12}, identity.ContentID{13}, identity.ContentID{14}); complete {
		t.Fatal("an incomplete target was constructed")
	}
	if _, sealed := calltarget.Family().Content(catalogLawFamily(), identity.ContentID{}); sealed {
		t.Fatal("a family sealed under no catalog")
	}
}

// An empty family is a published fact: the program emitted no call target,
// which is different from the family not being published at all.
func TestEmptyCallTargetFamilyIsPublished(t *testing.T) {
	frozen, catalog := sealCatalogLaw(t, nil)
	count, published := calltarget.Family().Count(&frozen, catalog)
	if !published || count != 0 {
		t.Fatalf("empty family width = %d (published %t)", count, published)
	}
	if _, held := calltarget.Family().At(&frozen, catalog, 0); held {
		t.Fatal("an empty family served a row")
	}
}

// A parent row names its children by offset and count, and the family answers
// that with the rows themselves rather than with a copy of them: the plane is
// sealed contiguously, so a span borrows it and costs no allocation however
// wide it is. A span that runs past the sealed family is not a short read, and
// the borrowed rows cannot be grown into the rows that follow them.
func TestCallTargetFamilySpanBorrowsTheSealedPlane(t *testing.T) {
	rows := catalogLawFamily()
	frozen, catalog := sealCatalogLaw(t, rows)

	span, held := calltarget.Family().Span(&frozen, catalog, 1, 1)
	if !held || len(span) != 1 || span[0] != rows[1] {
		t.Fatalf("span = %+v (held %t), want the second emitted row", span, held)
	}
	if cap(span) != len(span) {
		t.Fatalf("span capacity = %d, want %d", cap(span), len(span))
	}
	whole, wholeHeld := calltarget.Family().Span(&frozen, catalog, 0, uint32(len(rows)))
	if !wholeHeld || len(whole) != len(rows) || &whole[1] != &span[0] {
		t.Fatal("two spans of one plane borrow one storage")
	}
	if empty, emptyHeld := calltarget.Family().Span(&frozen, catalog, uint32(len(rows)), 0); !emptyHeld || len(empty) != 0 {
		t.Fatal("the empty span at the end of the plane is a span of no rows")
	}
	if _, past := calltarget.Family().Span(&frozen, catalog, 1, uint32(len(rows))); past {
		t.Fatal("a span past the sealed family borrowed rows")
	}
	foreign, derived := identity.DeriveContentID("cold-law/foreign-span-catalog", nil)
	if !derived {
		t.Fatal("foreign catalog")
	}
	if _, held := calltarget.Family().Span(&frozen, foreign, 0, 1); held {
		t.Fatal("a foreign catalog's axis borrowed rows")
	}

	var borrowed []calltarget.Target
	allocations := testing.AllocsPerRun(1000, func() {
		borrowed, _ = calltarget.Family().Span(&frozen, catalog, 0, uint32(len(rows)))
	})
	if allocations != 0 {
		t.Fatalf("span allocations = %v, want 0", allocations)
	}
	if len(borrowed) != len(rows) {
		t.Fatalf("borrowed %d rows, want %d", len(borrowed), len(rows))
	}
}

// A cold axis of one catalog reads nothing out of another catalog's
// publication, which is the property the derived catalog identity exists for.
func TestCatalogAxisOfAnotherCatalogReadsNothing(t *testing.T) {
	frozen, _ := sealCatalogLaw(t, catalogLawFamily())
	foreign, derived := identity.DeriveContentID("cold-law/foreign-catalog", nil)
	if !derived {
		t.Fatal("foreign catalog")
	}
	if _, status := snapshot.ReadFrozen(&frozen, calltarget.Family().Axis(foreign), programfamily.Ordinal(0)); status != snapshot.ReadInvalid {
		t.Fatalf("a foreign catalog's axis reported %v", status)
	}
	if _, published := calltarget.Family().Count(&frozen, foreign); published {
		t.Fatal("a foreign catalog reported a family width")
	}
}

// The child package is the complete source of declarations. This law
// enumerates that one manifest directly, so a family added later cannot be
// omitted from a handwritten census or silently reuse a slot/name.
func TestProgramCatalogManifestIsCompleteAndDense(t *testing.T) {
	manifest := programcatalog.Manifest()
	if len(manifest) != programcatalog.DefinitionCount() {
		t.Fatalf("manifest length = %d, definition count = %d", len(manifest), programcatalog.DefinitionCount())
	}
	slots := make(map[uint32]string, len(manifest))
	names := make(map[string]uint32, len(manifest))
	for index, definition := range manifest {
		if !definition.Valid() || definition.Name() == "" {
			t.Fatalf("manifest definition %d is invalid", index)
		}
		if definition.Slot() != uint32(index) {
			t.Fatalf("manifest definition %d has slot %d", index, definition.Slot())
		}
		if held, taken := slots[definition.Slot()]; taken {
			t.Fatalf("%s and %s both claim slot %d", held, definition.Name(), definition.Slot())
		}
		if held, taken := names[definition.Name()]; taken {
			t.Fatalf("%s is declared at slots %d and %d", definition.Name(), held, definition.Slot())
		}
		slots[definition.Slot()] = definition.Name()
		names[definition.Name()] = definition.Slot()
		if got, ok := programcatalog.DefinitionAt(index); !ok || got != definition {
			t.Fatalf("DefinitionAt(%d) did not return manifest definition", index)
		}
	}
	if _, ok := programcatalog.DefinitionAt(-1); ok {
		t.Fatal("negative manifest index was accepted")
	}
	if _, ok := programcatalog.DefinitionAt(len(manifest)); ok {
		t.Fatal("manifest length index was accepted")
	}
}

// Every canonical Family binding names its child-catalog definition. Keeping
// this table beside the manifest law makes the typed binding explicit without
// repeating slot arithmetic or family names.
func TestProgramFamilyAccessorsBindCatalogDefinitions(t *testing.T) {
	bindings := []struct {
		accessor string
		got      programcatalog.Definition
		want     programcatalog.Definition
	}{
		{"CallTargetFamily", calltarget.Family().Definition(), programcatalog.CallTarget()},
		{"HeapAllocationFamily", heapallocation.AllocationFamily().Definition(), programcatalog.HeapAllocation()},
		{"HeapFieldFamily", heapallocation.FieldFamily().Definition(), programcatalog.HeapField()},
		{"ValuesFamily", ValuesFamily().Definition(), programcatalog.Values()},
		{"ValuesMemberFamily", ValuesMemberFamily().Definition(), programcatalog.ValuesMember()},
		{"HeapIndexFamily", heapindex.Family().Definition(), programcatalog.HeapIndex()},
		{"ExactScalarSummaryFamily", ExactScalarSummaryFamily().Definition(), programcatalog.ExactScalarSummary()},
		{"ArithmeticSummaryFamily", ArithmeticSummaryFamily().Definition(), programcatalog.ArithmeticSummary()},
		{"UnarySummaryFamily", UnarySummaryFamily().Definition(), programcatalog.UnarySummary()},
		{"PointFamily", PointFamily().Definition(), programcatalog.Point()},
		{"PointDecisionFamily", PointDecisionFamily().Definition(), programcatalog.PointDecision()},
		{"CallFamily", CallFamily().Definition(), programcatalog.Call()},
		{"CallOperandFamily", CallOperandFamily().Definition(), programcatalog.CallOperand()},
		{"CallArgumentFamily", CallArgumentFamily().Definition(), programcatalog.CallArgument()},
		{"CallTypeArgumentFamily", CallTypeArgumentFamily().Definition(), programcatalog.CallTypeArgument()},
		{"EnvironmentEdgeFamily", EnvironmentEdgeFamily().Definition(), programcatalog.EnvironmentEdge()},
		{"EnvironmentResetFamily", EnvironmentResetFamily().Definition(), programcatalog.EnvironmentReset()},
		{"StaticTypeValueFamily", StaticTypeValueFamily().Definition(), programcatalog.StaticTypeValue()},
		{"StaticExpressionFamily", StaticExpressionFamily().Definition(), programcatalog.StaticExpression()},
		{"RegionFamily", RegionFamily().Definition(), programcatalog.Region()},
		{"RegionMemberFamily", RegionMemberFamily().Definition(), programcatalog.RegionMember()},
		{"WTOEventFamily", WTOEventFamily().Definition(), programcatalog.WTOEvent()},
		{"BodyFamily", BodyFamily().Definition(), programcatalog.Body()},
		{"BodyEntryFamily", BodyEntryFamily().Definition(), programcatalog.BodyEntry()},
		{"BodyRootFamily", BodyRootFamily().Definition(), programcatalog.BodyRoot()},
		{"OutcomeFamily", OutcomeFamily().Definition(), programcatalog.Outcome()},
		{"OutcomeReturnValueFamily", OutcomeReturnValueFamily().Definition(), programcatalog.OutcomeReturnValue()},
		{"OutcomePointFamily", OutcomePointFamily().Definition(), programcatalog.OutcomePoint()},
		{"FunctionBoundaryFamily", FunctionBoundaryFamily().Definition(), programcatalog.FunctionBoundary()},
		{"FunctionFormalFamily", FunctionFormalFamily().Definition(), programcatalog.FunctionFormal()},
		{"FunctionVarargFamily", FunctionVarargFamily().Definition(), programcatalog.FunctionVararg()},
		{"FunctionCaptureFamily", FunctionCaptureFamily().Definition(), programcatalog.FunctionCapture()},
		{"StaticInputFamily", StaticInputFamily().Definition(), programcatalog.StaticInput()},
		{"LocalTransferFamily", LocalTransferFamily().Definition(), programcatalog.LocalTransfer()},
		{"LocalTransferWriteFamily", LocalTransferWriteFamily().Definition(), programcatalog.LocalTransferWrite()},
		{"OccurrenceFamily", OccurrenceFamily().Definition(), programcatalog.Occurrence()},
		{"OccurrencePointFamily", OccurrencePointFamily().Definition(), programcatalog.OccurrencePoint()},
		{"OccurrenceInputFamily", OccurrenceInputFamily().Definition(), programcatalog.OccurrenceInput()},
		{"RuleOccurrenceFamily", RuleOccurrenceFamily().Definition(), programcatalog.RuleOccurrence()},
		{"CallResultFamily", CallResultFamily().Definition(), programcatalog.CallResult()},
		{"ModuleImportFamily", ModuleImportFamily().Definition(), programcatalog.ModuleImport()},
		{"ModuleRequestFamily", ModuleRequestFamily().Definition(), programcatalog.ModuleRequest()},
		{"ModuleEntryFamily", ModuleEntryFamily().Definition(), programcatalog.ModuleEntry()},
		{"ModuleEntryRootCellFamily", ModuleEntryRootCellFamily().Definition(), programcatalog.ModuleEntryRootCell()},
		{"ModuleEntryRootFunctionFamily", ModuleEntryRootFunctionFamily().Definition(), programcatalog.ModuleEntryRootFunction()},
		{"ModuleEntryMemberFamily", ModuleEntryMemberFamily().Definition(), programcatalog.ModuleEntryMember()},
		{"CallResultSlotFamily", CallResultSlotFamily().Definition(), programcatalog.CallResultSlot()},
	}
	// The remainder are bound by the sibling planes that own them - lifecycle,
	// heap, static type and diagnostics - rather than by this table.
	const boundElsewhere = 23
	if len(bindings)+boundElsewhere != programcatalog.DefinitionCount() {
		t.Fatalf("composed accessor bindings = %d, manifest definitions = %d", len(bindings)+boundElsewhere, programcatalog.DefinitionCount())
	}
	for _, binding := range bindings {
		if binding.got != binding.want {
			t.Fatalf("%s bound slot/name %d/%s, want %d/%s", binding.accessor,
				binding.got.Slot(), binding.got.Name(), binding.want.Slot(), binding.want.Name())
		}
	}
}

func catalogLawProgram(t *testing.T) Program {
	t.Helper()
	frozen, catalog := sealCatalogLaw(t, catalogLawFamily())
	schema := catalogLawSchema(t)
	artifact, artifactOK := identity.DeriveContentID("cold-law/artifact", nil)
	programID, programOK := identity.DeriveContentID("cold-law/program", nil)
	entry, entryOK := identity.DeriveContentID("cold-law/entry", nil)
	if !artifactOK || !programOK || !entryOK {
		t.Fatal("program identities")
	}
	row, ok := New(frozen, artifact, programID, schema, entry)
	if !ok {
		t.Fatal("New")
	}
	if row.catalogID != catalog {
		t.Fatal("New carried a catalog other than CatalogID(SchemaID)")
	}
	return row
}

func TestNewProgramCarriesCatalogWithoutRehash(t *testing.T) {
	row := catalogLawProgram(t)
	if !row.Available() {
		t.Fatal("New program is unavailable")
	}
	got, ok := row.catalog()
	if !ok || got != row.catalogID {
		t.Fatal("catalog()")
	}
	allocs := testing.AllocsPerRun(1000, func() {
		if !row.Available() {
			t.Fatal("available")
		}
		if _, held := row.catalog(); !held {
			t.Fatal("catalog")
		}
	})
	if allocs != 0 {
		t.Fatalf("carried catalog allocated %.2f", allocs)
	}
}

func TestNewProgramRefusesMismatchedFrozenSchema(t *testing.T) {
	frozen, _ := sealCatalogLaw(t, catalogLawFamily())
	other, ok := identity.DeriveContentID("cold-law/other-runtime-schema", nil)
	artifact, artifactOK := identity.DeriveContentID("cold-law/artifact", nil)
	programID, programOK := identity.DeriveContentID("cold-law/program", nil)
	entry, entryOK := identity.DeriveContentID("cold-law/entry", nil)
	if !ok || !artifactOK || !programOK || !entryOK {
		t.Fatal("identities")
	}
	if _, sealed := New(frozen, artifact, programID, other, entry); sealed {
		t.Fatal("New accepted a Frozen whose schema is not CatalogID(SchemaID)")
	}
}
