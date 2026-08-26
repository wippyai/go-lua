package summary_test

import (
	"testing"

	heapsummary "github.com/wippyai/go-lua/analysis/domain/heap/relation/summary"
	placementsummary "github.com/wippyai/go-lua/analysis/domain/placement/relation/summary"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
)

type allocationSummaryJudgment struct{}

func (allocationSummaryJudgment) Available() bool { return true }

func (allocationSummaryJudgment) Evaluate(argument placementsummary.PlacementSummaryAllocationArgument, emitter *relbindgen.Emitter[placementsummary.AllocationRow]) outcome.Code {
	metadata, present, ok := argument.MetadataAt(0)
	if !ok || !present {
		return outcome.Refused
	}
	fact, factPresent, ok := argument.PlacementFacts.At(0)
	if !ok || !factPresent {
		return outcome.Refused
	}
	evidence, ok := placementdomain.NewAllocationEvidence(metadata.ID(), placementdomain.AllocationKindTable, fact)
	if !ok {
		return outcome.Refused
	}
	row, ok := placementsummary.NewAllocationRow(metadata.ID(), fact, evidence)
	if !ok || !emitter.Put(row) {
		return outcome.Refused
	}
	return outcome.Produced
}

// absentAllocationSummaryJudgment verifies the same keyed output path when a
// semantic operation has no child payload to publish. The optional output
// contract must carry this as one coherent proven-absent row, not as a zero
// AllocationRow or an omitted column.
type absentAllocationSummaryJudgment struct{}

func (absentAllocationSummaryJudgment) Available() bool { return true }

func (absentAllocationSummaryJudgment) Evaluate(argument placementsummary.PlacementSummaryAllocationArgument, emitter *relbindgen.Emitter[placementsummary.AllocationRow]) outcome.Code {
	_, present, ok := argument.MetadataAt(0)
	if !ok || !present || !emitter.PutAbsent() {
		return outcome.Refused
	}
	return outcome.Produced
}

type allocationSummaryBindingFixture struct {
	place harness.Place

	allocationIDType   model.TypeID
	sourceType         model.TypeID
	heapRootType       model.TypeID
	factType           model.TypeID
	evidenceType       model.TypeID
	outputEvidenceType model.TypeID

	allocationIDColumn   model.ColumnID
	sourceColumn         model.ColumnID
	heapRootColumn       model.ColumnID
	factColumn           model.ColumnID
	evidenceColumn       model.ColumnID
	extraOutputIDColumn  model.ColumnID
	outputFactColumn     model.ColumnID
	outputEvidenceColumn model.ColumnID

	allocationID *relbindgen.Column[identity.ContentID]
	source       *relbindgen.Column[heapsummary.Source]
	heapRoot     *relbindgen.Column[heapdomain.Value]
	fact         *relbindgen.Column[placementdomain.Fact]
	evidence     *relbindgen.Column[suspension.Evidence]
	output       placementsummary.AllocationColumns
	columns      placementsummary.PlacementSummaryAllocationColumns
	operation    signature.Signature
}

func summaryContentID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func newAllocationSummaryBindingFixture(t *testing.T) allocationSummaryBindingFixture {
	t.Helper()
	place := harness.New(t, "allocation-summary-row")
	fixture := allocationSummaryBindingFixture{place: place}
	fixture.allocationIDType = place.TypeID(t, "type/allocation-id")
	fixture.sourceType = place.TypeID(t, "type/allocation-source")
	fixture.heapRootType = place.TypeID(t, "type/heap-root")
	fixture.factType = place.TypeID(t, "type/placement-fact")
	fixture.evidenceType = place.TypeID(t, "type/suspension-evidence")

	fixture.allocationIDColumn = place.Column(t, "column/allocation-id")
	fixture.sourceColumn = place.Column(t, "column/allocation-source")
	fixture.heapRootColumn = place.Column(t, "column/heap-root")
	fixture.factColumn = place.Column(t, "column/placement-fact")
	fixture.evidenceColumn = place.Column(t, "column/suspension-evidence")
	fixture.extraOutputIDColumn = place.Column(t, "column/extra-output-allocation-id")
	fixture.outputFactColumn = place.Column(t, "column/output-placement-fact")
	fixture.outputEvidenceColumn = place.Column(t, "column/output-evidence")

	fixture.allocationID = harness.NewColumn[identity.ContentID](t, fixture.allocationIDType, "store/allocation-id", 1)
	fixture.source = harness.NewColumn[heapsummary.Source](t, fixture.sourceType, "store/allocation-source", 1)
	fixture.heapRoot = harness.NewColumn[heapdomain.Value](t, fixture.heapRootType, "store/heap-root", 1)
	fixture.fact = harness.NewColumn[placementdomain.Fact](t, fixture.factType, "store/placement-fact", 1)
	fixture.evidence = harness.NewColumn[suspension.Evidence](t, fixture.evidenceType, "store/suspension-evidence", 1)
	outputFact := harness.NewColumn[placementdomain.Fact](t, fixture.factType, "store/output-placement-fact", 1)
	fixture.outputEvidenceType = place.TypeID(t, "type/allocation-evidence")
	outputEvidence := harness.NewColumn[placementdomain.AllocationEvidence](t, fixture.outputEvidenceType, "store/output-evidence", 1)
	var ok bool
	fixture.output, ok = placementsummary.NewAllocationColumns(outputFact, outputEvidence)
	if !ok {
		t.Fatal("allocation output columns")
	}
	fixture.columns, ok = placementsummary.NewPlacementSummaryAllocationColumns(
		fixture.allocationID, fixture.source, fixture.heapRoot, fixture.fact, fixture.evidence, fixture.output,
	)
	if !ok {
		t.Fatal("allocation summary columns")
	}

	cardinality, ok := model.NewCardinality(model.BoundedMany, 1)
	if !ok {
		t.Fatal("bounded-many cardinality")
	}
	fixture.operation = fixture.seal(t, "operation/allocation-summary", fixture.defaultInputs(t), signature.ProduceOptional, cardinality)
	return fixture
}

func (fixture allocationSummaryBindingFixture) defaultInputs(t *testing.T) []signature.Input {
	t.Helper()
	return fixture.inputs(t, [...]signature.PresenceContract{
		signature.RequirePresent,
		signature.RequirePresent,
		signature.AllowMissing,
		signature.AllowMissing,
		signature.AllowMissing,
	}, nil)
}

func (fixture allocationSummaryBindingFixture) inputs(t *testing.T, presence [5]signature.PresenceContract, deliveries []signature.Delivery) []signature.Input {
	t.Helper()
	columns := [...]model.ColumnID{
		fixture.allocationIDColumn,
		fixture.sourceColumn,
		fixture.heapRootColumn,
		fixture.factColumn,
		fixture.evidenceColumn,
	}
	types := [...]model.TypeID{
		fixture.allocationIDType,
		fixture.sourceType,
		fixture.heapRootType,
		fixture.factType,
		fixture.evidenceType,
	}
	result := make([]signature.Input, len(columns))
	for index := range columns {
		delivery := signature.Delivery{}
		if deliveries != nil {
			delivery = deliveries[index]
		} else {
			var ok bool
			delivery, ok = signature.NewCompleteSpanDelivery(fixture.place.Denominator.Key())
			if !ok {
				t.Fatal("complete delivery")
			}
		}
		result[index] = signature.Input{
			Relation: fixture.place.Relation, Column: columns[index], Type: types[index], Presence: presence[index],
			Delivery: delivery, Denominator: fixture.place.Denominator,
		}
	}
	return result
}

func (fixture allocationSummaryBindingFixture) seal(t *testing.T, label string, inputs []signature.Input, presence signature.PresenceContract, cardinality model.Cardinality) signature.Signature {
	t.Helper()
	return fixture.place.Seal(t, label, inputs, []signature.Output{
		{Relation: fixture.place.Relation, Column: fixture.outputFactColumn, Type: fixture.factType, Presence: presence, Denominator: fixture.place.Denominator},
		{Relation: fixture.place.Relation, Column: fixture.outputEvidenceColumn, Type: fixture.outputEvidenceType, Presence: presence, Denominator: fixture.place.Denominator},
	}, cardinality, outcome.Produced, outcome.Refused)
}

func allocationSummarySource() heapsummary.Source {
	return heapsummary.Source{Program: heapsummary.ProgramOrigin{
		Module: summaryContentID(21), ProgramID: summaryContentID(22), AllocationID: summaryContentID(23),
		Kind: heapdomain.AllocationTable, Form: heapdomain.AllocationFormClosed,
	}}
}

func (fixture allocationSummaryBindingFixture) presentSlots(t *testing.T) []binding.Slot {
	t.Helper()
	row := fixture.place.Rows[0]
	allocationID := row.Content()
	idToken, ok := fixture.allocationID.Encode(fixture.place.Issuer, allocationID)
	if !ok {
		t.Fatal("encode allocation ID")
	}
	sourceToken, ok := fixture.source.Encode(fixture.place.Issuer, allocationSummarySource())
	if !ok {
		t.Fatal("encode allocation source")
	}
	heapToken, ok := fixture.heapRoot.Encode(fixture.place.Issuer, heapdomain.Value{})
	if !ok {
		t.Fatal("encode heap root")
	}
	factToken, ok := fixture.fact.Encode(fixture.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode placement fact")
	}
	evidenceToken, ok := fixture.evidence.Encode(fixture.place.Issuer, suspension.EvidenceProven)
	if !ok {
		t.Fatal("encode suspension evidence")
	}
	return []binding.Slot{
		harness.SpanSlot(t, []binding.Cell{fixture.place.Cell(t, fixture.allocationIDColumn, row, fixture.allocationIDType, idToken)}),
		harness.SpanSlot(t, []binding.Cell{fixture.place.Cell(t, fixture.sourceColumn, row, fixture.sourceType, sourceToken)}),
		harness.SpanSlot(t, []binding.Cell{fixture.place.Cell(t, fixture.heapRootColumn, row, fixture.heapRootType, heapToken)}),
		harness.SpanSlot(t, []binding.Cell{fixture.place.Cell(t, fixture.factColumn, row, fixture.factType, factToken)}),
		harness.SpanSlot(t, []binding.Cell{fixture.place.Cell(t, fixture.evidenceColumn, row, fixture.evidenceType, evidenceToken)}),
	}
}

func TestPlacementSummaryAllocationBindingUsesPhysicalMetadataPair(t *testing.T) {
	fixture := newAllocationSummaryBindingFixture(t)
	factory, ok := placementsummary.BindPlacementSummaryAllocation(fixture.operation, allocationSummaryJudgment{}, fixture.columns, fixture.place.Refusal)
	if !ok {
		t.Fatal("bind allocation summary")
	}
	worker := fixture.place.Worker(t, factory, fixture.operation)
	slots := fixture.presentSlots(t)
	destinationSpan, ok := slots[0].Span()
	if !ok {
		t.Fatal("allocation summary destination span")
	}
	destination, ok := binding.NewSpanDestination(destinationSpan)
	if !ok {
		t.Fatal("allocation summary destination")
	}
	bufferValue, ok := binding.NewProposalBuffer(fixture.operation, fixture.place.Fence, []binding.DenominatorWitness{fixture.place.Witness}, fixture.place.Scope, destination)
	if !ok {
		t.Fatal("allocation summary buffer")
	}
	buffer := &bufferValue
	result := worker.Evaluate(fixture.place.Frame(t, slots...), buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 2 {
		t.Fatalf("allocation summary = %v with %d proposals, want Fact+AllocationEvidence row", result.Code, batch.Len())
	}
	for index := 0; index < batch.Len(); index++ {
		proposal, proposalOK := batch.At(index)
		if !proposalOK || !proposal.Presence().Is(model.Present) {
			t.Fatalf("proposal %d = %#v, want present", index, proposal)
		}
	}
}

func TestPlacementSummaryAllocationBindingPublishesCoherentAbsence(t *testing.T) {
	fixture := newAllocationSummaryBindingFixture(t)
	factory, ok := placementsummary.BindPlacementSummaryAllocation(fixture.operation, absentAllocationSummaryJudgment{}, fixture.columns, fixture.place.Refusal)
	if !ok {
		t.Fatal("bind allocation summary absence")
	}
	worker := fixture.place.Worker(t, factory, fixture.operation)
	slots := fixture.presentSlots(t)
	destinationSpan, ok := slots[0].Span()
	if !ok {
		t.Fatal("allocation summary absence destination span")
	}
	destination, ok := binding.NewSpanDestination(destinationSpan)
	if !ok {
		t.Fatal("allocation summary absence destination")
	}
	bufferValue, ok := binding.NewProposalBuffer(fixture.operation, fixture.place.Fence, []binding.DenominatorWitness{fixture.place.Witness}, fixture.place.Scope, destination)
	if !ok {
		t.Fatal("allocation summary absence buffer")
	}
	buffer := &bufferValue
	result := worker.Evaluate(fixture.place.Frame(t, slots...), buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 2 {
		t.Fatalf("allocation summary absence = %v with %d proposals, want two jointly absent columns", result.Code, batch.Len())
	}
	for index := 0; index < batch.Len(); index++ {
		proposal, proposalOK := batch.At(index)
		if !proposalOK || !proposal.Presence().Is(model.ProvenAbsent) || proposal.Value().Available() {
			t.Fatalf("proposal %d = %#v, want proven absence without payload", index, proposal)
		}
	}
}

func TestPlacementSummaryAllocationBindingRejectsShapeDrift(t *testing.T) {
	fixture := newAllocationSummaryBindingFixture(t)
	cardinality, ok := model.NewCardinality(model.BoundedMany, 1)
	if !ok {
		t.Fatal("bounded-many cardinality")
	}
	cases := []struct {
		name      string
		operation signature.Signature
	}{
		{
			name: "metadata presence is sparse",
			operation: fixture.seal(t, "operation/summary-sparse-metadata", fixture.inputs(t, [...]signature.PresenceContract{
				signature.AllowMissing, signature.RequirePresent, signature.AllowMissing, signature.AllowMissing, signature.AllowMissing,
			}, nil), signature.ProduceOptional, cardinality),
		},
		{
			name: "heap root is scalar",
			operation: func() signature.Signature {
				inputs := fixture.defaultInputs(t)
				delivery, deliveryOK := signature.NewScalarDelivery()
				if !deliveryOK {
					t.Fatal("scalar delivery")
				}
				inputs[2].Delivery = delivery
				return fixture.seal(t, "operation/summary-scalar-root", inputs, signature.ProduceOptional, cardinality)
			}(),
		},
		{
			name:      "output is not optional",
			operation: fixture.seal(t, "operation/summary-present-output", fixture.defaultInputs(t), signature.ProducePresent, cardinality),
		},
		{
			name: "cardinality is single",
			operation: func() signature.Signature {
				single, singleOK := model.NewCardinality(model.ExactlyOne, 0)
				if !singleOK {
					t.Fatal("exactly-one cardinality")
				}
				return fixture.seal(t, "operation/summary-single-output", fixture.defaultInputs(t), signature.ProduceOptional, single)
			}(),
		},
		{
			name: "extra heap key slot",
			operation: func() signature.Signature {
				inputs := append([]signature.Input(nil), fixture.defaultInputs(t)...)
				inputs = append(inputs, inputs[2])
				return fixture.seal(t, "operation/summary-extra-key", inputs, signature.ProduceOptional, cardinality)
			}(),
		},
		{
			name: "third allocation ID output",
			operation: func() signature.Signature {
				inputs := fixture.defaultInputs(t)
				outputs := fixture.operation.Outputs()
				outputs = append(outputs, signature.Output{
					Relation: fixture.place.Relation, Column: fixture.extraOutputIDColumn,
					Type: fixture.allocationIDType, Presence: signature.ProduceOptional,
					Denominator: fixture.place.Denominator,
				})
				return fixture.sealOutputs(t, "operation/summary-third-allocation-id", inputs, outputs, signature.ProduceOptional, cardinality)
			}(),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if factory, admitted := placementsummary.BindPlacementSummaryAllocation(testCase.operation, allocationSummaryJudgment{}, fixture.columns, fixture.place.Refusal); admitted || factory != nil {
				t.Fatal("shape drift admitted by allocation summary binding")
			}
		})
	}
}

func (fixture allocationSummaryBindingFixture) sealOutputs(t *testing.T, label string, inputs []signature.Input, outputs []signature.Output, presence signature.PresenceContract, cardinality model.Cardinality) signature.Signature {
	t.Helper()
	return fixture.place.Seal(t, label, inputs, outputs, cardinality, outcome.Produced, outcome.Refused)
}
