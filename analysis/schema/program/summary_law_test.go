package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func summaryLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, derived := identity.DeriveContentID("cold-law/summary/"+name, nil)
	if !derived {
		t.Fatalf("derive %s", name)
	}
	return id
}

func summaryLawRows(t *testing.T) ([]ExactScalarSummary, []ArithmeticSummary, []UnarySummary) {
	t.Helper()
	occurrence := summaryLawID(t, "occurrence")
	body := summaryLawID(t, "body")
	left, leftOK := NewExactScalarSummary(occurrence, summaryLawID(t, "left"), body, ExactScalarSummaryLeft, SummaryLiteral{Kind: 2, Integer: 10})
	right, rightOK := NewExactScalarSummary(occurrence, summaryLawID(t, "right"), body, ExactScalarSummaryRight, SummaryLiteral{Kind: 2, Integer: 5})
	arithmetic, arithmeticOK := NewArithmeticSummary(occurrence, body, SummaryOperator(5), NumericRepresentationInteger, NumericRepresentationInteger, NumericRepresentationInteger, ArithmeticDivisorNone)
	unary, unaryOK := NewUnarySummary(summaryLawID(t, "unary-occurrence"), body, summaryLawID(t, "point"), SummaryOperator(1), NumericRepresentationInteger, NumericRepresentationInteger)
	if !leftOK || !rightOK || !arithmeticOK || !unaryOK {
		t.Fatal("summary constructor rejected declared rows")
	}
	return []ExactScalarSummary{left, right}, []ArithmeticSummary{arithmetic}, []UnarySummary{unary}
}

func sealSummaryLaw(t *testing.T) (snapshot.Frozen, identity.ContentID, []ExactScalarSummary, []ArithmeticSummary, []UnarySummary) {
	t.Helper()
	runtimeSchema := summaryLawID(t, "runtime-schema")
	catalog, derived := CatalogID(runtimeSchema)
	if !derived {
		t.Fatal("cold catalog")
	}
	exact, arithmetic, unary := summaryLawRows(t)
	exactContent, exactSealed := ExactScalarSummaryFamily().Content(exact, catalog)
	arithmeticContent, arithmeticSealed := ArithmeticSummaryFamily().Content(arithmetic, catalog)
	unaryContent, unarySealed := UnarySummaryFamily().Content(unary, catalog)
	if !exactSealed || !arithmeticSealed || !unarySealed {
		t.Fatal("summary content rejected declared rows")
	}
	builder := snapshot.NewFrozen(catalog, identity.StoreID(11))
	for _, sealed := range []bool{
		CallTargetFamily().Put(&builder, nil, catalog),
		HeapAllocationFamily().Put(&builder, nil, catalog),
		HeapFieldFamily().Put(&builder, nil, catalog),
		ValuesFamily().Put(&builder, nil, catalog),
		ValuesMemberFamily().Put(&builder, nil, catalog),
		HeapIndexFamily().Put(&builder, nil, catalog),
	} {
		if !sealed {
			t.Fatal("empty preceding cold family")
		}
	}
	if err := snapshot.PutFrozenColumn(&builder, ExactScalarSummaryFamily().Axis(catalog), exactContent); err != nil {
		t.Fatalf("put exact scalar column: %v", err)
	}
	if err := snapshot.PutFrozenColumn(&builder, ArithmeticSummaryFamily().Axis(catalog), arithmeticContent); err != nil {
		t.Fatalf("put arithmetic column: %v", err)
	}
	if err := snapshot.PutFrozenColumn(&builder, UnarySummaryFamily().Axis(catalog), unaryContent); err != nil {
		t.Fatalf("put unary column: %v", err)
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal summary columns: %v", err)
	}
	return frozen, catalog, exact, arithmetic, unary
}

// Every summary family is a dense cold column: its width and ordinal reads
// come from the same Frozen publication, with no second slice authority.
func TestSummaryColumnsReadBackThroughProgram(t *testing.T) {
	frozen, _, exact, arithmetic, unary := sealSummaryLaw(t)
	program := Program{Frozen: frozen, ArtifactID: summaryLawID(t, "artifact"), ProgramID: summaryLawID(t, "program"), SchemaID: summaryLawID(t, "runtime-schema")}
	if !program.Available() {
		t.Fatal("program rejected sealed summary publication")
	}
	exactCount, exactPublished := program.ExactScalarSummaryCount()
	arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
	unaryCount, unaryPublished := program.UnarySummaryCount()
	if !exactPublished || !arithmeticPublished || !unaryPublished || exactCount != len(exact) || arithmeticCount != len(arithmetic) || unaryCount != len(unary) {
		t.Fatalf("column widths = %d/%v %d/%v %d/%v", exactCount, exactPublished, arithmeticCount, arithmeticPublished, unaryCount, unaryPublished)
	}
	for index, want := range exact {
		got, held := program.ExactScalarSummaryAt(index)
		if !held || got.ID() != want.ID() || got.SubjectID() != want.SubjectID() {
			t.Fatalf("exact ordinal %d drifted", index)
		}
	}
	gotArithmetic, arithmeticHeld := program.ArithmeticSummaryAt(0)
	if !arithmeticHeld || gotArithmetic.ID() != arithmetic[0].ID() {
		t.Fatal("arithmetic ordinal did not read the published row")
	}
	gotUnary, unaryHeld := program.UnarySummaryAt(0)
	if !unaryHeld || gotUnary.ID() != unary[0].ID() {
		t.Fatal("unary ordinal did not read the published row")
	}
	if _, held := program.ExactScalarSummaryAt(exactCount); held {
		t.Fatal("past-end exact ordinal reported a row")
	}
	if _, held := program.ArithmeticSummaryAt(-1); held {
		t.Fatal("negative arithmetic ordinal reported a row")
	}
	foreign := summaryLawID(t, "foreign-catalog")
	if _, published := ExactScalarSummaryFamily().Count(&frozen, foreign); published {
		t.Fatal("foreign catalog reported the summary family")
	}
	if _, status := snapshot.ReadFrozen(&frozen, ExactScalarSummaryFamily().Axis(foreign), Ordinal(0)); status != snapshot.ReadInvalid {
		t.Fatalf("foreign exact axis reported %v", status)
	}
}

// The cold carrier validates only declared primitive shape. Operation
// interpretation remains a compiler/domain concern, so an opaque nonzero
// operator is retained without importing Program vocabulary into schema.
func TestSummaryCarrierDoesNotReinterpretOperator(t *testing.T) {
	arithmetic, ok := NewArithmeticSummary(summaryLawID(t, "occurrence"), summaryLawID(t, "body"), SummaryOperator(99), NumericRepresentationInteger, NumericRepresentationFloat, NumericRepresentationNumber, ArithmeticDivisorNone)
	if !ok || !arithmetic.Available() || arithmetic.Operator() != SummaryOperator(99) {
		t.Fatal("cold carrier reinterpreted an opaque operator")
	}
	if _, ok := NewArithmeticSummary(identity.ContentID{}, summaryLawID(t, "body"), SummaryOperator(1), NumericRepresentationInteger, NumericRepresentationInteger, NumericRepresentationInteger, ArithmeticDivisorNone); ok {
		t.Fatal("cold carrier accepted an unavailable identity")
	}
}
