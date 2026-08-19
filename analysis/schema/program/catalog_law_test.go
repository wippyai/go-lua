package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
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
	catalog, derived := CatalogID(runtime)
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
	second, secondDerived := CatalogID(other)
	if !secondDerived || second == catalog {
		t.Fatal("two declaration catalogs derived one cold catalog")
	}
	if _, derived := CatalogID(identity.ContentID{}); derived {
		t.Fatal("an unavailable runtime schema derived a cold catalog")
	}
}

func catalogLawFamily() []CallTarget {
	return []CallTarget{
		{Allocation: identity.ContentID{1}, Body: identity.ContentID{2}, Context: identity.ContentID{3}, Function: identity.ContentID{4}, Formal: identity.ContentID{5}},
		{Allocation: identity.ContentID{6}, Body: identity.ContentID{7}, Context: identity.ContentID{8}, Function: identity.ContentID{9}, Formal: identity.ContentID{10}},
	}
}

func sealCatalogLaw(t *testing.T, rows []CallTarget) (snapshot.Frozen, identity.ContentID) {
	t.Helper()
	catalog, derived := CatalogID(catalogLawSchema(t))
	if !derived {
		t.Fatal("cold catalog")
	}
	content, sealed := CallTargetFamily().Content(rows, catalog)
	if !sealed {
		t.Fatal("call target content")
	}
	builder := snapshot.NewFrozen(catalog, identity.StoreID(7))
	if err := snapshot.PutFrozenColumn(&builder, CallTargetFamily().Axis(catalog), content); err != nil {
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

	count, published := CallTargetFamily().Count(&frozen, catalog)
	if !published || count != len(rows) {
		t.Fatalf("family width = %d (published %t), want %d", count, published, len(rows))
	}
	for index, want := range rows {
		got, held := CallTargetFamily().At(&frozen, catalog, index)
		if !held || got != want {
			t.Fatalf("ordinal %d read %+v, emitted %+v", index, got, want)
		}
	}
	if _, held := CallTargetFamily().At(&frozen, catalog, len(rows)); held {
		t.Fatal("an ordinal past the family read a row")
	}
	if _, held := CallTargetFamily().At(&frozen, catalog, -1); held {
		t.Fatal("a negative ordinal read a row")
	}
}

// An ordinal past the sealed family is a proven absence, not ignorance: the
// column publishes the family's own ordinal range as its key universe.
func TestCallTargetFamilyProvesItsOwnBound(t *testing.T) {
	rows := catalogLawFamily()
	frozen, catalog := sealCatalogLaw(t, rows)

	if _, status := snapshot.ReadFrozen(&frozen, CallTargetFamily().Axis(catalog), Ordinal(len(rows)-1)); status != snapshot.ReadHit {
		t.Fatalf("last ordinal reported %v", status)
	}
	if _, status := snapshot.ReadFrozen(&frozen, CallTargetFamily().Axis(catalog), Ordinal(len(rows))); status == snapshot.ReadHit {
		t.Fatal("an ordinal past the family reported a hit")
	}
}

// A family the compiler could not prove seals nothing: a program either
// proved every target it emitted or it did not compile.
func TestCallTargetContentRejectsAnUnprovenRow(t *testing.T) {
	catalog, derived := CatalogID(catalogLawSchema(t))
	if !derived {
		t.Fatal("cold catalog")
	}
	rows := append(catalogLawFamily(), CallTarget{Allocation: identity.ContentID{11}})
	if _, sealed := CallTargetFamily().Content(rows, catalog); sealed {
		t.Fatal("an incomplete target sealed into the family")
	}
	if _, sealed := CallTargetFamily().Content(catalogLawFamily(), identity.ContentID{}); sealed {
		t.Fatal("a family sealed under no catalog")
	}
}

// An empty family is a published fact: the program emitted no call target,
// which is different from the family not being published at all.
func TestEmptyCallTargetFamilyIsPublished(t *testing.T) {
	frozen, catalog := sealCatalogLaw(t, nil)
	count, published := CallTargetFamily().Count(&frozen, catalog)
	if !published || count != 0 {
		t.Fatalf("empty family width = %d (published %t)", count, published)
	}
	if _, held := CallTargetFamily().At(&frozen, catalog, 0); held {
		t.Fatal("an empty family served a row")
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
	if _, status := snapshot.ReadFrozen(&frozen, CallTargetFamily().Axis(foreign), Ordinal(0)); status != snapshot.ReadInvalid {
		t.Fatalf("a foreign catalog's axis reported %v", status)
	}
	if _, published := CallTargetFamily().Count(&frozen, foreign); published {
		t.Fatal("a foreign catalog reported a family width")
	}
}

// Every declared family owns a slot and a name no other family owns. A slot
// is half of the address every consumer holds, so two families sharing one
// slot would make the second column unpublishable and the first
// unaddressable. The catalog states the law over the whole declaration set
// rather than trusting each family's own arithmetic.
func TestProgramFamilySlotsAndNamesAreDistinct(t *testing.T) {
	declared := []struct {
		slot uint32
		name string
	}{
		{CallTargetFamily().slot, CallTargetFamily().name},
		{HeapAllocationFamily().slot, HeapAllocationFamily().name},
		{HeapFieldFamily().slot, HeapFieldFamily().name},
		{ValuesFamily().slot, ValuesFamily().name},
		{ValuesMemberFamily().slot, ValuesMemberFamily().name},
		{HeapIndexFamily().slot, HeapIndexFamily().name},
		{ExactScalarSummaryFamily().slot, ExactScalarSummaryFamily().name},
		{ArithmeticSummaryFamily().slot, ArithmeticSummaryFamily().name},
		{UnarySummaryFamily().slot, UnarySummaryFamily().name},
		{PointFamily().slot, PointFamily().name},
		{PointDecisionFamily().slot, PointDecisionFamily().name},
		{CallFamily().slot, CallFamily().name},
		{CallOperandFamily().slot, CallOperandFamily().name},
		{CallArgumentFamily().slot, CallArgumentFamily().name},
		{CallTypeArgumentFamily().slot, CallTypeArgumentFamily().name},
		{EnvironmentEdgeFamily().slot, EnvironmentEdgeFamily().name},
		{EnvironmentResetFamily().slot, EnvironmentResetFamily().name},
		{StaticTypeValueFamily().slot, StaticTypeValueFamily().name},
		{StaticExpressionFamily().slot, StaticExpressionFamily().name},
		{RegionFamily().slot, RegionFamily().name},
		{RegionMemberFamily().slot, RegionMemberFamily().name},
		{WTOEventFamily().slot, WTOEventFamily().name},
		{BodyFamily().slot, BodyFamily().name},
		{BodyEntryFamily().slot, BodyEntryFamily().name},
		{BodyRootFamily().slot, BodyRootFamily().name},
		{OutcomeFamily().slot, OutcomeFamily().name},
		{OutcomeReturnValueFamily().slot, OutcomeReturnValueFamily().name},
		{OutcomePointFamily().slot, OutcomePointFamily().name},
		{FunctionBoundaryFamily().slot, FunctionBoundaryFamily().name},
		{FunctionFormalFamily().slot, FunctionFormalFamily().name},
		{FunctionVarargFamily().slot, FunctionVarargFamily().name},
		{FunctionCaptureFamily().slot, FunctionCaptureFamily().name},
		{StaticInputFamily().slot, StaticInputFamily().name},
		{LocalTransferFamily().slot, LocalTransferFamily().name},
		{LocalTransferWriteFamily().slot, LocalTransferWriteFamily().name},
		{OccurrenceFamily().slot, OccurrenceFamily().name},
		{OccurrencePointFamily().slot, OccurrencePointFamily().name},
		{OccurrenceInputFamily().slot, OccurrenceInputFamily().name},
		{RuleOccurrenceFamily().slot, RuleOccurrenceFamily().name},
	}
	slots := make(map[uint32]string, len(declared))
	names := make(map[string]uint32, len(declared))
	for _, family := range declared {
		if family.name == "" {
			t.Fatalf("slot %d is declared without a name", family.slot)
		}
		if held, taken := slots[family.slot]; taken {
			t.Fatalf("%s and %s both claim slot %d", held, family.name, family.slot)
		}
		if held, taken := names[family.name]; taken {
			t.Fatalf("%s is declared at slots %d and %d", family.name, held, family.slot)
		}
		slots[family.slot] = family.name
		names[family.name] = family.slot
	}
}

// The publication is total over the catalog: sealing every declared family,
// including the empty ones, publishes one column per declaration. A family
// whose slot collided with another's would fail here rather than at the first
// program that emitted rows into both.
func TestProgramPublicationSealsEveryDeclaredFamily(t *testing.T) {
	catalog, derived := CatalogID(catalogLawSchema(t))
	if !derived {
		t.Fatal("catalog identity")
	}
	frozen, sealed := Publication{}.Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("the empty publication did not seal every declared family")
	}
	if _, published := PointFamily().Count(&frozen, catalog); !published {
		t.Fatal("point family is not published")
	}
	if _, published := WTOEventFamily().Count(&frozen, catalog); !published {
		t.Fatal("event family is not published")
	}
	if _, published := OutcomePointFamily().Count(&frozen, catalog); !published {
		t.Fatal("outcome point family is not published")
	}
	if _, published := LocalTransferWriteFamily().Count(&frozen, catalog); !published {
		t.Fatal("local-transfer write family is not published")
	}
}
