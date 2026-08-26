package summary_test

import (
	"testing"

	placementsummary "github.com/wippyai/go-lua/analysis/domain/placement/relation/summary"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// parentNoSelection is used only to exercise the typed address-only decoder;
// it never reads a QuerySite payload.
type parentNoSelection struct{}

func (parentNoSelection) Available() bool { return true }

func (parentNoSelection) Evaluate(placementsummary.PlacementSummaryParentArgument, *relbindgen.Emitter[placementsummary.ParentAnswer]) outcome.Code {
	return outcome.NoSelection
}

type parentBindingFixture struct {
	place harness.Place

	queryType      model.TypeID
	allocationType model.TypeID
	factType       model.TypeID
	evidenceType   model.TypeID
	answerType     model.TypeID

	queryColumn      model.ColumnID
	allocationColumn model.ColumnID
	factColumn       model.ColumnID
	evidenceColumn   model.ColumnID
	answerColumn     model.ColumnID

	query      *relbindgen.Column[identity.ContentID]
	allocation *relbindgen.Column[identity.ContentID]
	fact       *relbindgen.Column[placementdomain.Fact]
	evidence   *relbindgen.Column[placementdomain.AllocationEvidence]
	answer     *relbindgen.Column[identity.ContentID]
	columns    placementsummary.PlacementSummaryParentColumns
	operation  signature.Signature
}

func newParentBindingFixture(t *testing.T) parentBindingFixture {
	t.Helper()
	place := harness.New(t, "query-site")
	fixture := parentBindingFixture{place: place}
	fixture.queryType = place.TypeID(t, "type/query-site-address")
	fixture.allocationType = place.TypeID(t, "type/allocation-id")
	fixture.factType = place.TypeID(t, "type/placement-fact")
	fixture.evidenceType = place.TypeID(t, "type/allocation-evidence")
	fixture.answerType = place.TypeID(t, "type/placement-schema-id")
	fixture.queryColumn = place.Column(t, "column/query-site-address")
	fixture.allocationColumn = place.Column(t, "column/allocation-id")
	fixture.factColumn = place.Column(t, "column/placement-fact")
	fixture.evidenceColumn = place.Column(t, "column/allocation-evidence")
	fixture.answerColumn = place.Column(t, "column/placement-schema-id")
	fixture.query = harness.NewColumn[identity.ContentID](t, fixture.queryType, "store/query-site", 1)
	fixture.allocation = harness.NewColumn[identity.ContentID](t, fixture.allocationType, "store/allocation-id", 1)
	fixture.fact = harness.NewColumn[placementdomain.Fact](t, fixture.factType, "store/placement-fact", 1)
	fixture.evidence = harness.NewColumn[placementdomain.AllocationEvidence](t, fixture.evidenceType, "store/allocation-evidence", 1)
	fixture.answer = harness.NewColumn[identity.ContentID](t, fixture.answerType, "store/placement-schema-id", 1)
	parentColumns, ok := placementsummary.NewParentColumns(fixture.answer)
	if !ok {
		t.Fatal("parent output columns")
	}
	fixture.columns, ok = placementsummary.NewPlacementSummaryParentColumns(
		fixture.queryColumn, fixture.queryType, fixture.allocation, fixture.fact, fixture.evidence, parentColumns,
	)
	if !ok {
		t.Fatal("parent columns")
	}
	optional, ok := model.NewCardinality(model.Optional, 0)
	if !ok {
		t.Fatal("parent cardinality")
	}
	complete, ok := signature.NewCompleteSpanDelivery(place.Denominator.Key())
	if !ok {
		t.Fatal("complete delivery")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	fixture.operation = place.Seal(t, "operation/placement-summary-parent", []signature.Input{
		{Relation: place.Relation, Column: fixture.queryColumn, Type: fixture.queryType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: place.Denominator},
		{Relation: place.Relation, Column: fixture.allocationColumn, Type: fixture.allocationType, Presence: signature.AllowMissing, Delivery: complete, Denominator: place.Denominator},
		{Relation: place.Relation, Column: fixture.factColumn, Type: fixture.factType, Presence: signature.AllowMissing, Delivery: complete, Denominator: place.Denominator},
		{Relation: place.Relation, Column: fixture.evidenceColumn, Type: fixture.evidenceType, Presence: signature.AllowMissing, Delivery: complete, Denominator: place.Denominator},
	}, []signature.Output{{
		Relation: place.Relation, Column: fixture.answerColumn, Type: fixture.answerType,
		Presence: signature.ProduceOpaque, Denominator: place.Denominator,
	}}, optional, outcome.Produced, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	return fixture
}

func (fixture parentBindingFixture) bind(t *testing.T) binding.Worker {
	t.Helper()
	factory, ok := placementsummary.BindPlacementSummaryParent(fixture.operation, parentNoSelection{}, fixture.columns, fixture.place.Refusal)
	if !ok {
		t.Fatal("bind parent")
	}
	return fixture.place.Worker(t, factory, fixture.operation)
}

func (fixture parentBindingFixture) querySlot(t *testing.T) binding.Slot {
	t.Helper()
	token, ok := fixture.query.Encode(fixture.place.Issuer, harness.Content(t, "query-site-payload-is-ignored"))
	if !ok {
		t.Fatal("query-site token")
	}
	return harness.ScalarSlot(t, parentSemanticCell(t, fixture.place, fixture.queryColumn, fixture.place.Rows[0], fixture.queryType, token, model.AuthenticatedOpaque))
}

func TestPlacementSummaryParentAcceptsAddressOnlyQuerySite(t *testing.T) {
	fixture := newParentBindingFixture(t)
	worker := fixture.bind(t)
	buffer := fixture.place.Buffer(t, fixture.operation)
	result := worker.Evaluate(fixture.place.Frame(t, fixture.querySlot(t),
		harness.SpanSlot(t, []binding.Cell{fixture.place.AbsentCell(t, fixture.allocationColumn, fixture.place.Rows[0], fixture.allocationType)}),
		harness.SpanSlot(t, []binding.Cell{fixture.place.AbsentCell(t, fixture.factColumn, fixture.place.Rows[0], fixture.factType)}),
		harness.SpanSlot(t, []binding.Cell{fixture.place.AbsentCell(t, fixture.evidenceColumn, fixture.place.Rows[0], fixture.evidenceType)}),
	), buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || result.Code != outcome.NoSelection || batch.Len() != 0 {
		t.Fatalf("address-only parent = %v sealed=%t rows=%d, want NoSelection/empty", result.Code, sealed, batch.Len())
	}
}

func TestPlacementSummaryParentRejectsShapeDrift(t *testing.T) {
	fixture := newParentBindingFixture(t)
	mutations := []struct {
		name   string
		mutate func([]signature.Input, []signature.Output, *model.Cardinality)
	}{
		{name: "query site is span", mutate: func(inputs []signature.Input, _ []signature.Output, _ *model.Cardinality) {
			inputs[0].Delivery, _ = signature.NewCompleteSpanDelivery(fixture.place.Denominator.Key())
		}},
		{name: "child denominator differs", mutate: func(inputs []signature.Input, _ []signature.Output, _ *model.Cardinality) {
			otherKey, ok := model.IssueKeyID(fixture.place.Relation, harness.Content(t, "other-denominator"))
			if !ok {
				t.Fatalf("other key")
			}
			other, ok := model.NewDenominatorRef(fixture.place.Relation, otherKey)
			if !ok {
				t.Fatalf("other denominator")
			}
			inputs[3].Denominator = other
		}},
		{name: "parent is required", mutate: func(_ []signature.Input, outputs []signature.Output, _ *model.Cardinality) {
			outputs[0].Presence = signature.ProduceOptional
		}},
		{name: "parent is exactly one", mutate: func(_ []signature.Input, _ []signature.Output, cardinality *model.Cardinality) {
			*cardinality, _ = model.NewCardinality(model.ExactlyOne, 0)
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			inputs := fixture.operation.Inputs()
			outputs := fixture.operation.Outputs()
			cardinality := fixture.operation.Cardinality()
			test.mutate(inputs, outputs, &cardinality)
			operation := sealParentShape(t, fixture.place, inputs, outputs, cardinality)
			if factory, admitted := placementsummary.BindPlacementSummaryParent(operation, parentNoSelection{}, fixture.columns, fixture.place.Refusal); admitted || factory != nil {
				t.Fatal("parent shape drift admitted")
			}
		})
	}
}

func sealParentShape(t *testing.T, place harness.Place, inputs []signature.Input, outputs []signature.Output, cardinality model.Cardinality) signature.Signature {
	t.Helper()
	codes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("parent outcomes")
	}
	sealed, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: place.OperationID(t, "parent-shape-mutated"), Version: 1},
		Fence:    signature.Fence{Owner: place.Owner, Schema: place.SchemaID}, Inputs: inputs, Outputs: outputs,
		Cardinality: cardinality, Outcomes: codes,
	})
	if !ok {
		t.Fatal("parent shape seal")
	}
	return sealed
}

// parentOperationFixture binds the real parent operation over the exact
// Placement schema used by the child-summary fixture. The test still uses a
// tiny one-row mounted denominator, so every hostile case remains a single
// bounded invocation rather than a corpus run.
type parentOperationFixture struct {
	base allocationSummaryOperationFixture

	queryType  model.TypeID
	query      *relbindgen.Column[identity.ContentID]
	queryCol   model.ColumnID
	answer     *relbindgen.Column[identity.ContentID]
	answerType model.TypeID
	answerCol  model.ColumnID

	columns   placementsummary.PlacementSummaryParentColumns
	operation signature.Signature
	worker    binding.Worker
}

func newParentOperationFixture(t *testing.T) parentOperationFixture {
	t.Helper()
	base := newAllocationSummaryOperationFixture(t)
	place := base.base.place
	fixture := parentOperationFixture{base: base}
	fixture.queryType = place.TypeID(t, "type/parent-query-site")
	fixture.queryCol = place.Column(t, "column/parent-query-site")
	fixture.query = harness.NewColumn[identity.ContentID](t, fixture.queryType, "store/parent-query-site", 1)
	fixture.answerType = place.TypeID(t, "type/parent-schema-id")
	fixture.answerCol = place.Column(t, "column/parent-schema-id")
	fixture.answer = harness.NewColumn[identity.ContentID](t, fixture.answerType, "store/parent-schema-id", 1)
	parentOutput, ok := placementsummary.NewParentColumns(fixture.answer)
	if !ok {
		t.Fatal("parent output")
	}
	fixture.columns, ok = placementsummary.NewPlacementSummaryParentColumns(
		fixture.queryCol, fixture.queryType,
		base.base.allocationID, base.base.output.Fact, base.base.output.Evidence,
		parentOutput,
	)
	if !ok {
		t.Fatal("parent columns")
	}
	optional, ok := model.NewCardinality(model.Optional, 0)
	if !ok {
		t.Fatal("parent cardinality")
	}
	complete, ok := signature.NewCompleteSpanDelivery(place.Denominator.Key())
	if !ok {
		t.Fatal("parent complete delivery")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("parent scalar delivery")
	}
	fixture.operation = place.Seal(t, "operation/placement-summary-parent-semantic", []signature.Input{
		{Relation: place.Relation, Column: fixture.queryCol, Type: fixture.queryType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: place.Denominator},
		// This compact semantic harness keeps the legacy two-column child
		// output and supplies the base QAllocation identity as the first
		// parent slot. The admission E2E exercises the honest three-column
		// child ABI (AllocationID, Fact, Evidence).
		{Relation: place.Relation, Column: base.base.allocationIDColumn, Type: base.base.allocationIDType, Presence: signature.AllowMissing, Delivery: complete, Denominator: place.Denominator},
		{Relation: place.Relation, Column: base.base.outputFactColumn, Type: base.base.factType, Presence: signature.AllowMissing, Delivery: complete, Denominator: place.Denominator},
		{Relation: place.Relation, Column: base.base.outputEvidenceColumn, Type: base.base.outputEvidenceType, Presence: signature.AllowMissing, Delivery: complete, Denominator: place.Denominator},
	}, []signature.Output{{
		Relation: place.Relation, Column: fixture.answerCol, Type: fixture.answerType,
		Presence: signature.ProduceOpaque, Denominator: place.Denominator,
	}}, optional, outcome.Produced, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	judgment, ok := placementsummary.NewPlacementSummaryParentOperation(base.schema)
	if !ok {
		t.Fatal("parent operation")
	}
	factory, ok := placementsummary.BindPlacementSummaryParent(fixture.operation, judgment, fixture.columns, place.Refusal)
	if !ok {
		t.Fatal("bind parent operation")
	}
	fixture.worker = place.Worker(t, factory, fixture.operation)
	return fixture
}

func (fixture parentOperationFixture) slots(t *testing.T, allocationKind, factKind, evidenceKind model.PresenceKind, foreignAllocation bool) []binding.Slot {
	t.Helper()
	place := fixture.base.base.place
	if fixture.base.schema.KeyCount() != 1 {
		t.Fatalf("parent fixture allocation width = %d, want one", fixture.base.schema.KeyCount())
	}
	key, ok := fixture.base.schema.KeyAt(0)
	if !ok {
		t.Fatal("parent allocation key")
	}
	allocationID, ok := key.ContentID()
	if !ok {
		t.Fatal("parent allocation identity")
	}
	encodedID := allocationID
	if foreignAllocation {
		encodedID = harness.Content(t, "foreign-parent-allocation")
	}
	fact := placementdomain.DefaultFact()
	evidence, ok := placementdomain.NewAllocationEvidence(allocationID, placementdomain.AllocationKindTable, fact)
	if !ok {
		t.Fatal("parent evidence")
	}
	queryToken, ok := fixture.query.Encode(place.Issuer, harness.Content(t, "parent-query-payload-ignored"))
	if !ok {
		t.Fatal("parent query token")
	}
	idToken, ok := fixture.base.base.allocationID.Encode(place.Issuer, encodedID)
	if !ok {
		t.Fatal("parent allocation token")
	}
	factToken, ok := fixture.base.base.output.Fact.Encode(place.Issuer, fact)
	if !ok {
		t.Fatal("parent fact token")
	}
	evidenceToken, ok := fixture.base.base.output.Evidence.Encode(place.Issuer, evidence)
	if !ok {
		t.Fatal("parent evidence token")
	}
	if allocationKind != model.Present && allocationKind != model.AuthenticatedOpaque {
		idToken = binding.ValueToken{}
	}
	if factKind != model.Present && factKind != model.AuthenticatedOpaque {
		factToken = binding.ValueToken{}
	}
	if evidenceKind != model.Present && evidenceKind != model.AuthenticatedOpaque {
		evidenceToken = binding.ValueToken{}
	}
	row := place.Rows[0]
	return []binding.Slot{
		harness.ScalarSlot(t, parentSemanticCell(t, place, fixture.queryCol, row, fixture.queryType, queryToken, model.AuthenticatedOpaque)),
		harness.SpanSlot(t, []binding.Cell{parentSemanticCell(t, place, fixture.base.base.allocationIDColumn, row, fixture.base.base.allocationIDType, idToken, allocationKind)}),
		harness.SpanSlot(t, []binding.Cell{parentSemanticCell(t, place, fixture.base.base.outputFactColumn, row, fixture.base.base.factType, factToken, factKind)}),
		harness.SpanSlot(t, []binding.Cell{parentSemanticCell(t, place, fixture.base.base.outputEvidenceColumn, row, fixture.base.base.outputEvidenceType, evidenceToken, evidenceKind)}),
	}
}

func parentSemanticCell(t *testing.T, place harness.Place, column model.ColumnID, row model.RowID, typeID model.TypeID, token binding.ValueToken, kind model.PresenceKind) binding.Cell {
	t.Helper()
	address, ok := place.Issuer.IssueCell(place.Witness, place.Scope, column, row)
	if !ok {
		t.Fatal("parent cell address")
	}
	presence, ok := model.NewPresence(kind)
	if !ok {
		t.Fatal("parent cell presence")
	}
	cell, ok := binding.NewCell(address, typeID, token, presence)
	if !ok {
		t.Fatal("parent cell")
	}
	return cell
}

func (fixture parentOperationFixture) evaluate(t *testing.T, allocationKind, factKind, evidenceKind model.PresenceKind, foreignAllocation bool) (outcome.Result, binding.ProposalBatch, bool) {
	t.Helper()
	place := fixture.base.base.place
	slots := fixture.slots(t, allocationKind, factKind, evidenceKind, foreignAllocation)
	destinationCell, ok := slots[0].At(0)
	if !ok {
		t.Fatal("parent destination cell")
	}
	destination, ok := binding.NewScalarDestination(destinationCell)
	if !ok {
		t.Fatal("parent destination")
	}
	buffer, ok := binding.NewProposalBuffer(fixture.operation, place.Fence, []binding.DenominatorWitness{place.Witness}, place.Scope, destination)
	if !ok {
		t.Fatal("parent operation buffer")
	}
	result := fixture.worker.Evaluate(place.Frame(t, slots...), &buffer)
	batch, sealed := buffer.Seal(result)
	return result, batch, sealed
}

func TestPlacementSummaryParentOperationPublishesOneExactAnswer(t *testing.T) {
	fixture := newParentOperationFixture(t)
	result, batch, sealed := fixture.evaluate(t, model.Present, model.Present, model.Present, false)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("parent present = %v sealed=%t rows=%d, want Produced/one", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || !proposal.Presence().Is(model.AuthenticatedOpaque) {
		t.Fatal("parent proposal is not authenticated opaque")
	}
	id, ok := fixture.answer.Decode(proposal.Value())
	if !ok || id != fixture.base.schema.ContentID() {
		t.Fatalf("parent schema ID = %x/%t, want %x", id, ok, fixture.base.schema.ContentID())
	}
	if proposal.Destination().Row().Content() != fixture.base.base.place.Rows[0].Content() {
		t.Fatal("parent answer moved away from QuerySite row")
	}
}

func TestPlacementSummaryParentOperationSettlesHostilePresence(t *testing.T) {
	fixture := newParentOperationFixture(t)
	cases := []struct {
		name                       string
		allocation, fact, evidence model.PresenceKind
		foreign                    bool
		want                       outcome.Code
		wantRows                   int
	}{
		{name: "all proven absent", allocation: model.ProvenAbsent, fact: model.ProvenAbsent, evidence: model.ProvenAbsent, want: outcome.NoSelection},
		{name: "mixed child columns", allocation: model.Present, fact: model.ProvenAbsent, evidence: model.Present, want: outcome.Refused},
		{name: "unproven missing", allocation: model.UnprovenMissing, fact: model.UnprovenMissing, evidence: model.UnprovenMissing, want: outcome.Refused},
		{name: "authenticated opaque", allocation: model.AuthenticatedOpaque, fact: model.AuthenticatedOpaque, evidence: model.AuthenticatedOpaque, want: outcome.Opaque},
		{name: "foreign child identity", allocation: model.Present, fact: model.Present, evidence: model.Present, foreign: true, want: outcome.Refused},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, batch, sealed := fixture.evaluate(t, test.allocation, test.fact, test.evidence, test.foreign)
			if !sealed || result.Code != test.want || batch.Len() != test.wantRows {
				t.Fatalf("parent %s = %v sealed=%t rows=%d, want %v/%d", test.name, result.Code, sealed, batch.Len(), test.want, test.wantRows)
			}
		})
	}
}
