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
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

// allocationSummaryOperationFixture uses the production Heap/Placement
// schema with the small ABI harness. All five inputs happen to have one row
// in this fixture, while the operation still validates the independent full
// Heap width before it derives containment.
type allocationSummaryOperationFixture struct {
	base      allocationSummaryBindingFixture
	world     relationfixture.Sealed
	schema    placementdomain.Schema
	operation signature.Signature
	worker    binding.Worker
}

func newAllocationSummaryOperationFixture(t *testing.T) allocationSummaryOperationFixture {
	t.Helper()
	base := newAllocationSummaryBindingFixture(t)
	world := relationfixture.New(t)
	schema, ok := placementdomain.NewSchema(world.Heap)
	if !ok {
		t.Fatal("Placement schema")
	}
	allocationKey, ok := schema.KeyAt(0)
	if !ok {
		t.Fatal("allocation key")
	}
	allocationID, ok := allocationKey.ContentID()
	if !ok {
		t.Fatal("allocation ID")
	}
	// The harness identities are deterministic, but its default row label is
	// intentionally unrelated to a production allocation. Mount the same
	// authenticated denominator with the exact sealed allocation identity so
	// keyed publication is tested at the real destination rather than being
	// rejected as a foreign row.
	base.place = harness.NewKeyed(t, []identity.ContentID{allocationID})
	cardinality, ok := model.NewCardinality(model.BoundedMany, uint32(schema.KeyCount()))
	if !ok || schema.KeyCount() == 0 {
		t.Fatal("allocation summary cardinality")
	}
	operation := sealAllocationSummaryOperation(t, base, "operation/allocation-summary-semantic", base.defaultInputs(t), signature.ProduceOptional, cardinality, outcome.Produced, outcome.Opaque, outcome.Refused)
	judgment, ok := placementsummary.NewPlacementSummaryAllocationOperation(schema)
	if !ok {
		t.Fatal("allocation summary operation")
	}
	factory, ok := placementsummary.BindPlacementSummaryAllocation(operation, judgment, base.columns, base.place.Refusal)
	if !ok {
		t.Fatal("bind allocation summary operation")
	}
	return allocationSummaryOperationFixture{
		base: base, world: world, schema: schema, operation: operation,
		worker: base.place.Worker(t, factory, operation),
	}
}

func sealAllocationSummaryOperation(t *testing.T, base allocationSummaryBindingFixture, label string, inputs []signature.Input, presence signature.PresenceContract, cardinality model.Cardinality, codes ...outcome.Code) signature.Signature {
	t.Helper()
	set, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("allocation summary outcome set")
	}
	sealed, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: base.place.OperationID(t, label), Version: 1},
		Fence:    signature.Fence{Owner: base.place.Owner, Schema: base.place.SchemaID},
		Inputs:   inputs,
		Outputs: []signature.Output{
			{Relation: base.place.Relation, Column: base.outputFactColumn, Type: base.factType, Presence: presence, Denominator: base.place.Denominator},
			{Relation: base.place.Relation, Column: base.outputEvidenceColumn, Type: base.outputEvidenceType, Presence: presence, Denominator: base.place.Denominator},
		},
		Cardinality: cardinality,
		Outcomes:    set,
	})
	if !ok {
		t.Fatal("seal allocation summary operation")
	}
	return sealed
}

func (fixture allocationSummaryOperationFixture) slots(t *testing.T, factKind, evidenceKind model.PresenceKind, malformedMetadata bool) []binding.Slot {
	t.Helper()
	if fixture.schema.KeyCount() != 1 || fixture.schema.Heap().KeyCount() != 1 {
		t.Fatalf("fixture schema widths = allocation=%d heap=%d, want one-row fixture", fixture.schema.KeyCount(), fixture.schema.Heap().KeyCount())
	}
	key, keyOK := fixture.schema.KeyAt(0)
	if !keyOK {
		t.Fatal("allocation key")
	}
	metadata, metadataOK := heapsummary.NewAllocationRow(fixture.world.Heap, key)
	if !metadataOK {
		t.Fatal("Heap allocation metadata")
	}
	if malformedMetadata {
		metadata.Source = heapsummary.Source{}
	}
	idToken, ok := fixture.base.allocationID.Encode(fixture.base.place.Issuer, metadata.ID())
	if !ok {
		t.Fatal("allocation ID token")
	}
	sourceToken, ok := fixture.base.source.Encode(fixture.base.place.Issuer, metadata.Source)
	if !ok {
		t.Fatal("allocation source token")
	}
	heapToken, ok := fixture.base.heapRoot.Encode(fixture.base.place.Issuer, fixture.world.Heap.Bottom())
	if !ok {
		t.Fatal("Heap root token")
	}
	factToken, ok := fixture.base.fact.Encode(fixture.base.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("Placement fact token")
	}
	evidenceToken, ok := fixture.base.evidence.Encode(fixture.base.place.Issuer, suspension.EvidenceProven)
	if !ok {
		t.Fatal("suspension evidence token")
	}
	row := fixture.base.place.Rows[0]
	if factKind != model.Present && factKind != model.AuthenticatedOpaque {
		factToken = binding.ValueToken{}
	}
	if evidenceKind != model.Present && evidenceKind != model.AuthenticatedOpaque {
		evidenceToken = binding.ValueToken{}
	}
	return []binding.Slot{
		harness.SpanSlot(t, []binding.Cell{summaryOperationCell(t, fixture.base.place, fixture.base.allocationIDColumn, row, fixture.base.allocationIDType, idToken, model.Present)}),
		harness.SpanSlot(t, []binding.Cell{summaryOperationCell(t, fixture.base.place, fixture.base.sourceColumn, row, fixture.base.sourceType, sourceToken, model.Present)}),
		harness.SpanSlot(t, []binding.Cell{summaryOperationCell(t, fixture.base.place, fixture.base.heapRootColumn, row, fixture.base.heapRootType, heapToken, model.Present)}),
		harness.SpanSlot(t, []binding.Cell{summaryOperationCell(t, fixture.base.place, fixture.base.factColumn, row, fixture.base.factType, factToken, factKind)}),
		harness.SpanSlot(t, []binding.Cell{summaryOperationCell(t, fixture.base.place, fixture.base.evidenceColumn, row, fixture.base.evidenceType, evidenceToken, evidenceKind)}),
	}
}

func summaryOperationCell(t *testing.T, place harness.Place, column model.ColumnID, row model.RowID, typeID model.TypeID, token binding.ValueToken, kind model.PresenceKind) binding.Cell {
	t.Helper()
	address, ok := place.Issuer.IssueCell(place.Witness, place.Scope, column, row)
	if !ok {
		t.Fatal("operation cell address")
	}
	presence, ok := model.NewPresence(kind)
	if !ok {
		t.Fatal("operation cell presence")
	}
	cell, ok := binding.NewCell(address, typeID, token, presence)
	if !ok {
		t.Fatal("operation cell")
	}
	return cell
}

func (fixture allocationSummaryOperationFixture) evaluate(t *testing.T, factKind, evidenceKind model.PresenceKind, malformedMetadata bool) (outcome.Result, binding.ProposalBatch, bool) {
	t.Helper()
	slots := fixture.slots(t, factKind, evidenceKind, malformedMetadata)
	destinationSpan, ok := slots[0].Span()
	if !ok {
		t.Fatal("operation destination span")
	}
	destination, ok := binding.NewSpanDestination(destinationSpan)
	if !ok {
		t.Fatal("operation destination")
	}
	buffer, ok := binding.NewProposalBuffer(fixture.operation, fixture.base.place.Fence, []binding.DenominatorWitness{fixture.base.place.Witness}, fixture.base.place.Scope, destination)
	if !ok {
		t.Fatal("operation buffer")
	}
	result := fixture.worker.Evaluate(fixture.base.place.Frame(t, slots...), &buffer)
	batch, sealed := buffer.Seal(result)
	return result, batch, sealed
}

func TestPlacementSummaryAllocationOperationPublishesCanonicalChild(t *testing.T) {
	fixture := newAllocationSummaryOperationFixture(t)
	result, batch, sealed := fixture.evaluate(t, model.Present, model.Present, false)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 2 {
		t.Fatalf("allocation summary operation = %v sealed=%t rows=%d, want Fact+AllocationEvidence child", result.Code, sealed, batch.Len())
	}
	for index := 0; index < batch.Len(); index++ {
		proposal, ok := batch.At(index)
		if !ok || !proposal.Presence().Is(model.Present) {
			t.Fatalf("child proposal %d = %#v, want present", index, proposal)
		}
	}
	key, ok := fixture.schema.KeyAt(0)
	if !ok {
		t.Fatal("allocation key")
	}
	id, ok := key.ContentID()
	if !ok {
		t.Fatal("allocation ID")
	}
	factProposal, ok := batch.At(0)
	if !ok {
		t.Fatal("Fact proposal")
	}
	fact, ok := fixture.base.output.Fact.Decode(factProposal.Value())
	if !ok || fact != placementdomain.DefaultFact() {
		t.Fatalf("published Fact = %#v/%t, want canonical default Fact", fact, ok)
	}
	evidenceProposal, ok := batch.At(1)
	if !ok {
		t.Fatal("AllocationEvidence proposal")
	}
	evidence, ok := fixture.base.output.Evidence.Decode(evidenceProposal.Value())
	if !ok || evidence.OwnerIdentity != id || !evidence.HasOwnerIdentity {
		t.Fatalf("published AllocationEvidence owner = %v/%t, want %v/true", evidence.OwnerIdentity, ok, id)
	}
}

func TestPlacementSummaryAllocationOperationPreservesSparseAbsence(t *testing.T) {
	fixture := newAllocationSummaryOperationFixture(t)
	result, batch, sealed := fixture.evaluate(t, model.ProvenAbsent, model.ProvenAbsent, false)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 2 {
		t.Fatalf("sparse allocation summary = %v sealed=%t rows=%d, want two jointly absent columns", result.Code, sealed, batch.Len())
	}
	for index := 0; index < batch.Len(); index++ {
		proposal, ok := batch.At(index)
		if !ok || !proposal.Presence().Is(model.ProvenAbsent) || proposal.Value().Available() {
			t.Fatalf("sparse proposal %d = %#v, want proven absence without payload", index, proposal)
		}
	}
}

func TestPlacementSummaryAllocationOperationRefusesUnfinishedOrMalformedInputs(t *testing.T) {
	fixture := newAllocationSummaryOperationFixture(t)
	missing, _, sealed := fixture.evaluate(t, model.UnprovenMissing, model.ProvenAbsent, false)
	if !sealed || missing.Code != outcome.Refused {
		t.Fatalf("unfinished Placement = %v sealed=%t, want refused", missing.Code, sealed)
	}
	malformed, _, sealed := fixture.evaluate(t, model.Present, model.ProvenAbsent, true)
	if !sealed || malformed.Code != outcome.Refused {
		t.Fatalf("malformed metadata = %v sealed=%t, want refused", malformed.Code, sealed)
	}
}

func TestPlacementSummaryAllocationOperationPropagatesAuthenticatedOpaque(t *testing.T) {
	fixture := newAllocationSummaryOperationFixture(t)
	result, batch, sealed := fixture.evaluate(t, model.AuthenticatedOpaque, model.ProvenAbsent, false)
	if !sealed || result.Code != outcome.Opaque || batch.Len() != 0 {
		t.Fatalf("opaque Placement = %v sealed=%t rows=%d, want opaque without rows", result.Code, sealed, batch.Len())
	}
}

func TestPlacementSummaryAllocationOperationRejectsInvalidAuthority(t *testing.T) {
	if operation, ok := placementsummary.NewPlacementSummaryAllocationOperation(placementdomain.Schema{}); ok || operation.Available() {
		t.Fatal("invalid Placement schema became an operation")
	}
	fixture := newAllocationSummaryOperationFixture(t)
	if fixture.operation.Digest() == (identity.ContentID{}) {
		t.Fatal("sealed operation lost its digest")
	}
}

var _ relbindgen.Operation[placementsummary.PlacementSummaryAllocationArgument, placementsummary.AllocationRow] = placementsummary.PlacementSummaryAllocationOperation{}

// Keep the declaration corpus linked in this law so a future deletion of the
// old summary family cannot silently leave this operation as an unregistered
// parallel mechanism.
var _ = relbind.Declared
