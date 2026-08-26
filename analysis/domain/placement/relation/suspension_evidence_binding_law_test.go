package relation_test

import (
	"testing"

	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

// TestPlacementSuspensionEvidenceDeclarationKeepsFoldIrreducible guards the
// sealed family declaration itself: the owner route relation may read a
// complete Value vector, but this evidence operation receives only scalar
// owner-issued consequences.
func TestPlacementSuspensionEvidenceDeclarationKeepsFoldIrreducible(t *testing.T) {
	corpus := relbind.Declared()
	var found *relbind.Family
	for index := range corpus.Families {
		family := &corpus.Families[index]
		if family.Census == "placement/suspension-evidence" {
			found = family
			break
		}
	}
	if found == nil {
		t.Fatal("corpus has no Placement suspension-evidence family")
	}
	if len(found.Inputs) != 5 || found.Result != "suspension-evidence" || len(found.Outputs) != 1 {
		t.Fatalf("evidence family shape = inputs=%d result=%q outputs=%d", len(found.Inputs), found.Result, len(found.Outputs))
	}
	for _, input := range found.Inputs {
		if input.Delivery != signature.ScalarDelivery {
			t.Fatalf("evidence fold input %q uses %v, want scalar", input.Field, input.Delivery)
		}
		if input.Payload == "value" || input.Payload == "value-summary" {
			t.Fatalf("evidence fold reopened Value vector through %q", input.Payload)
		}
	}
}

type suspensionEvidenceBindingFixture struct {
	place     harness.Place
	operation signature.Signature
	worker    binding.Worker

	candidateColumn *relbindgen.Column[suspension.MountedSubjectLiveness]
	summaryColumn   *relbindgen.Column[suspension.SourceSummary]
	routeColumn     *relbindgen.Column[heapdomain.Key]
	routeTagColumn  *relbindgen.Column[uint64]
	evidenceColumn  *relbindgen.Column[suspension.Evidence]

	candidateAddress model.ColumnID
	summaryAddress   model.ColumnID
	routeAddress     model.ColumnID
	routeTagAddress  model.ColumnID
	evidenceAddress  model.ColumnID
	outputAddress    model.ColumnID

	candidateType model.TypeID
	summaryType   model.TypeID
	routeType     model.TypeID
	routeTagType  model.TypeID
	evidenceType  model.TypeID
}

func newSuspensionEvidenceBindingFixture(t *testing.T) suspensionEvidenceBindingFixture {
	t.Helper()
	place := harness.New(t,
		"row/suspension-evidence-candidate",
		"row/suspension-evidence-summary",
		"row/suspension-evidence-route",
		"row/suspension-evidence-tag",
		"row/suspension-evidence-selected",
	)
	candidateType := place.TypeID(t, "type/suspension-evidence-candidate")
	summaryType := place.TypeID(t, "type/suspension-source-summary")
	routeType := place.TypeID(t, "type/suspension-evidence-route")
	routeTagType := place.TypeID(t, "type/suspension-evidence-route-tag")
	evidenceType := place.TypeID(t, "type/suspension-evidence")
	candidateColumn := harness.NewColumn[suspension.MountedSubjectLiveness](t, candidateType, "store/suspension-evidence-candidate", reserve)
	summaryColumn := harness.NewColumn[suspension.SourceSummary](t, summaryType, "store/suspension-source-summary", reserve)
	routeColumn := harness.NewColumn[heapdomain.Key](t, routeType, "store/suspension-evidence-route", reserve)
	routeTagColumn := harness.NewColumn[uint64](t, routeTagType, "store/suspension-evidence-route-tag", reserve)
	evidenceColumn := harness.NewColumn[suspension.Evidence](t, evidenceType, "store/suspension-evidence", reserve)
	columns, ok := placementrelation.NewPlacementSuspensionEvidenceColumns(routeColumn, candidateColumn, summaryColumn, evidenceColumn, routeTagColumn)
	if !ok {
		t.Fatal("suspension-evidence owner columns")
	}
	candidateAddress := place.Column(t, "column/suspension-evidence-candidate")
	summaryAddress := place.Column(t, "column/suspension-source-summary")
	routeAddress := place.Column(t, "column/suspension-evidence-route")
	routeTagAddress := place.Column(t, "column/suspension-evidence-route-tag")
	evidenceAddress := place.Column(t, "column/suspension-evidence-selected")
	outputAddress := place.Column(t, "column/suspension-evidence-output")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("suspension-evidence cardinality")
	}
	operation := place.Seal(t, "operation/placement-suspension-evidence",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, summaryAddress, summaryType, place.Denominator),
			harness.ScalarInput(t, place.Relation, routeAddress, routeType, place.Denominator),
			harness.ScalarInput(t, place.Relation, routeTagAddress, routeTagType, place.Denominator),
			harness.ScalarInput(t, place.Relation, evidenceAddress, evidenceType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: outputAddress, Type: evidenceType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused,
	)
	factory, ok := placementrelation.BindPlacementSuspensionEvidence(operation, placementrelation.PlacementSuspensionEvidenceOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement suspension evidence")
	}
	return suspensionEvidenceBindingFixture{
		place: place, operation: operation, worker: place.Worker(t, factory, operation),
		candidateColumn: candidateColumn, summaryColumn: summaryColumn, routeColumn: routeColumn,
		routeTagColumn: routeTagColumn, evidenceColumn: evidenceColumn,
		candidateAddress: candidateAddress, summaryAddress: summaryAddress, routeAddress: routeAddress,
		routeTagAddress: routeTagAddress, evidenceAddress: evidenceAddress, outputAddress: outputAddress,
		candidateType: candidateType, summaryType: summaryType, routeType: routeType,
		routeTagType: routeTagType, evidenceType: evidenceType,
	}
}

func (fixture suspensionEvidenceBindingFixture) evaluate(t *testing.T, candidate suspension.MountedSubjectLiveness, summary suspension.SourceSummary, route heapdomain.Key, routeTag uint64, selected suspension.Evidence, candidatePresent bool) (outcome.Result, binding.ProposalBatch, bool) {
	t.Helper()
	candidateToken, ok := fixture.candidateColumn.Encode(fixture.place.Issuer, candidate)
	if !ok {
		t.Fatal("encode suspension-evidence candidate")
	}
	summaryToken, ok := fixture.summaryColumn.Encode(fixture.place.Issuer, summary)
	if !ok {
		t.Fatal("encode suspension source summary")
	}
	routeToken, ok := fixture.routeColumn.Encode(fixture.place.Issuer, route)
	if !ok {
		t.Fatal("encode suspension-evidence route")
	}
	routeTagToken, ok := fixture.routeTagColumn.Encode(fixture.place.Issuer, routeTag)
	if !ok {
		t.Fatal("encode suspension-evidence route tag")
	}
	evidenceToken, ok := fixture.evidenceColumn.Encode(fixture.place.Issuer, selected)
	if !ok {
		t.Fatal("encode suspension evidence")
	}
	row := fixture.place.Rows[0]
	candidateCell := fixture.place.Cell(t, fixture.candidateAddress, row, fixture.candidateType, candidateToken)
	if !candidatePresent {
		candidateCell = fixture.place.AbsentCell(t, fixture.candidateAddress, row, fixture.candidateType)
	}
	frame := fixture.place.Frame(t,
		harness.ScalarSlot(t, candidateCell),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.summaryAddress, fixture.place.Rows[1], fixture.summaryType, summaryToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeAddress, fixture.place.Rows[2], fixture.routeType, routeToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTagAddress, fixture.place.Rows[3], fixture.routeTagType, routeTagToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.evidenceAddress, fixture.place.Rows[4], fixture.evidenceType, evidenceToken)),
	)
	buffer := fixture.place.BufferAt(t, fixture.operation, fixture.place.Rows[4])
	result := fixture.worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	return result, batch, sealed
}

// TestPlacementSuspensionEvidenceBindsOwnerScalars proves that the evidence
// family receives the route's scalar consequence and publishes only the
// evidence owner state. The complete Value vector is absent from this frame;
// it was consumed by the owner-derived route relation before binding.
func TestPlacementSuspensionEvidenceBindsOwnerScalars(t *testing.T) {
	world := relationfixture.New(t)
	candidate := suspensionBindingCandidate(t)
	fixture := newSuspensionEvidenceBindingFixture(t)
	want, reduction := suspension.SuspensionEvidenceFold(candidate, suspension.SourceSummaryKnown, world.Root, 1, suspension.EvidenceRefuted)
	if reduction != structure.Concrete || want != suspension.EvidenceRefuted {
		t.Fatalf("real SuspensionEvidenceFold = %v/%v, want Refuted/Concrete", want, reduction)
	}
	result, batch, sealed := fixture.evaluate(t, candidate, suspension.SourceSummaryKnown, world.Root, 1, suspension.EvidenceRefuted, true)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("suspension evidence binding = %v sealed=%t rows=%d, want one produced row", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != fixture.place.Rows[4] || proposal.Destination().Column() != fixture.outputAddress {
		t.Fatal("suspension evidence did not publish at its final selected Factor row")
	}
	if proposal.Destination().Row() == fixture.place.Rows[0] {
		t.Fatal("suspension evidence published at candidate row instead of selected Factor row")
	}
	published, ok := fixture.evidenceColumn.Decode(proposal.Value())
	if !ok || published != want {
		t.Fatalf("suspension evidence = %v, want %v", published, want)
	}

	// A summary marked Unknown is an owner-issued scalar widening consequence;
	// the binding carries it without reopening or comparing any Value token.
	unknown, reduction := suspension.SuspensionEvidenceFold(candidate, suspension.SourceSummaryUnknown, world.Root, 1, suspension.EvidenceRefuted)
	if reduction != structure.Concrete || unknown != suspension.EvidenceUnknown {
		t.Fatalf("unknown-summary fold = %v/%v, want Unknown/Concrete", unknown, reduction)
	}
	result, batch, sealed = fixture.evaluate(t, candidate, suspension.SourceSummaryUnknown, world.Root, 1, suspension.EvidenceRefuted, true)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("unknown-summary binding = %v sealed=%t rows=%d, want one produced row", result.Code, sealed, batch.Len())
	}
	proposal, ok = batch.At(0)
	if !ok {
		t.Fatal("unknown-summary binding produced no proposal")
	}
	published, ok = fixture.evidenceColumn.Decode(proposal.Value())
	if !ok || published != unknown {
		t.Fatalf("unknown-summary evidence = %v, want %v", published, unknown)
	}
}

// TestPlacementSuspensionEvidenceRefusesUnissuedInputs keeps malformed owner
// values out of the evidence axis. Refusal is the only valid result for an
// invalid summary, an unissued route, or an absent mounted candidate.
func TestPlacementSuspensionEvidenceRefusesUnissuedInputs(t *testing.T) {
	world := relationfixture.New(t)
	candidate := suspensionBindingCandidate(t)
	fixture := newSuspensionEvidenceBindingFixture(t)
	checkRefused := func(name string, supplied suspension.MountedSubjectLiveness, summary suspension.SourceSummary, route heapdomain.Key, candidatePresent bool) {
		t.Helper()
		result, batch, sealed := fixture.evaluate(t, supplied, summary, route, 1, suspension.EvidenceRefuted, candidatePresent)
		if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
			t.Fatalf("%s = %v sealed=%t rows=%d, want refused without rows", name, result.Code, sealed, batch.Len())
		}
	}
	checkRefused("invalid source summary", candidate, suspension.SourceSummaryInvalid, world.Root, true)
	checkRefused("unissued route", candidate, suspension.SourceSummaryKnown, heapdomain.Key{}, true)
	checkRefused("absent candidate", candidate, suspension.SourceSummaryKnown, world.Root, false)
}
