package apply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// These laws use only the generic semantic ABI.  They intentionally bypass
// mount construction and install an already authenticated witness directly in
// the package-private Executor so the Apply boundary can be tested without a
// domain or a second fake mount protocol.
type applyFixture struct {
	owner       model.OwnerID
	relation    model.RelationID
	input       model.ColumnID
	output      model.ColumnID
	typeID      model.TypeID
	key         model.KeyID
	denominator model.DenominatorRef
	schema      model.SchemaID
	operation   signature.Signature
	refusal     model.RefusalID
	runtime     binding.Fence
	issuer      binding.Issuer
	scope       binding.ScopeToken
	witness     binding.DenominatorWitness
	rows        []model.RowID
	lineage     model.LineageRef
	lineageAuth lineage.Authority
}

type scriptedWorker struct {
	calls  int
	action func(binding.Frame, *binding.ProposalBuffer) outcome.Result
}

func (worker *scriptedWorker) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if worker == nil {
		return outcome.Result{}
	}
	worker.calls++
	if worker.action == nil {
		return outcome.Result{}
	}
	return worker.action(frame, buffer)
}

func testContent(label string) identity.ContentID {
	value, ok := identity.DeriveContentID("engine/relation/apply/law", []byte(label))
	if !ok {
		panic("test content")
	}
	return value
}

func newApplyFixture(t *testing.T, cardinality model.Cardinality, inputPresence signature.PresenceContract, outputPresence signature.PresenceContract, rows int) applyFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(testContent("owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, testContent("relation"))
	if !ok {
		t.Fatal("relation")
	}
	input, ok := model.IssueColumnID(relation, testContent("column/input"))
	if !ok {
		t.Fatal("input column")
	}
	output, ok := model.IssueColumnID(relation, testContent("column/output"))
	if !ok {
		t.Fatal("output column")
	}
	typeID, ok := model.IssueTypeID(owner, testContent("type/value"))
	if !ok {
		t.Fatal("type")
	}
	key, ok := model.IssueKeyID(relation, testContent("key/denominator"))
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	schema, ok := model.IssueSchemaID(owner, testContent("schema"))
	if !ok {
		t.Fatal("schema")
	}
	operationID, ok := model.IssueOperationID(owner, testContent("operation"))
	if !ok {
		t.Fatal("operation")
	}
	refusal, ok := model.IssueRefusalID(owner, testContent("refusal"))
	if !ok {
		t.Fatal("refusal")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("delivery")
	}
	codes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	operation, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schema},
		Inputs: []signature.Input{{
			Relation: relation, Column: input, Type: typeID, Presence: inputPresence,
			Delivery: delivery, Denominator: denominator,
		}},
		Outputs:     []signature.Output{{Relation: relation, Column: output, Type: typeID, Presence: outputPresence, Denominator: denominator}},
		Cardinality: cardinality,
		Outcomes:    codes,
	})
	if !ok {
		t.Fatal("signature")
	}
	runtime, ok := binding.NewFence(schema, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("runtime fence")
	}
	issuer, ok := binding.NewIssuer(runtime)
	if !ok {
		t.Fatal("issuer")
	}
	lineageOwner, ok := model.IssueOwnerID(testContent("lineage-owner"))
	if !ok {
		t.Fatal("lineage owner")
	}
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	lineageAuth, ok := lineageFactory.Bind(runtime)
	if !ok || lineageAuth == nil {
		t.Fatal("lineage authority")
	}
	rowIDs := make([]model.RowID, rows)
	for index := range rowIDs {
		rowIDs[index], ok = model.IssueRowID(relation, testContent("row/"+string(rune('a'+index))))
		if !ok {
			t.Fatal("row")
		}
	}
	// Source provenance mirrors the mounted RowLineage projection: the
	// relation owner issues the row's content atom, and the dedicated lineage
	// authority imports that foreign atom. Tests must not mint a same-owner
	// authority reference that was never admitted into its arena.
	lineageValue, ok := model.IssueLineageRef(owner, rowIDs[0].Content())
	if !ok {
		t.Fatal("lineage")
	}
	scope, ok := issuer.IssueScope(testContent("scope/formula"))
	if !ok {
		t.Fatal("scope")
	}
	membership, ok := binding.NewMembershipView(relation, rowIDs)
	if !ok {
		t.Fatal("membership")
	}
	witness, ok := issuer.IssueDenominator(denominator, membership, testContent("denominator/evidence"))
	if !ok {
		t.Fatal("witness")
	}
	return applyFixture{
		owner: owner, relation: relation, input: input, output: output, typeID: typeID,
		key: key, denominator: denominator, schema: schema, operation: operation,
		refusal: refusal, runtime: runtime, issuer: issuer, scope: scope,
		witness: witness, rows: rowIDs, lineage: lineageValue, lineageAuth: lineageAuth,
	}
}

func executorFor(fixture applyFixture, worker binding.Worker) Executor {
	return Executor{data: &executorData{
		operation: fixture.operation, worker: worker, fence: fixture.runtime,
		destinations: []binding.DenominatorWitness{fixture.witness},
		scope:        fixture.scope, lineage: fixture.lineageAuth,
	}}
}

func frameFor(t *testing.T, fixture applyFixture, presence model.PresenceKind) binding.Frame {
	t.Helper()
	address, ok := fixture.issuer.IssueCell(fixture.witness, fixture.scope, fixture.input, fixture.rows[0])
	if !ok {
		t.Fatal("input address")
	}
	status, ok := model.NewPresence(presence)
	if !ok {
		t.Fatal("input presence")
	}
	value := binding.ValueToken{}
	if presence == model.Present || presence == model.AuthenticatedOpaque {
		value, ok = fixture.issuer.IssueValue(fixture.typeID, testContent("input/value"))
		if !ok {
			t.Fatal("input value")
		}
	}
	cell, ok := binding.NewCell(address, fixture.typeID, value, status)
	if !ok {
		t.Fatal("input cell")
	}
	slot, ok := binding.NewScalarSlot(cell)
	if !ok {
		t.Fatal("input slot")
	}
	frame, ok := binding.NewFrame(fixture.scope, slot)
	if !ok {
		t.Fatal("frame")
	}
	return frame
}

func result(code outcome.Code) outcome.Result {
	value, ok := outcome.NewResult(code, model.RefusalID{})
	if !ok {
		panic("test outcome")
	}
	return value
}

func refusal(fixture applyFixture) outcome.Result {
	value, ok := outcome.NewResult(outcome.Refused, fixture.refusal)
	if !ok {
		panic("test refusal")
	}
	return value
}

func presentProposal(t *testing.T, fixture applyFixture, buffer *binding.ProposalBuffer, row int, presence model.PresenceKind) (binding.Proposal, bool) {
	t.Helper()
	issuer, ok := binding.NewIssuer(buffer.Fence())
	if !ok || row < 0 || row >= len(fixture.rows) {
		return binding.Proposal{}, false
	}
	witness, ok := buffer.DestinationWitness(fixture.denominator)
	if !ok {
		return binding.Proposal{}, false
	}
	destination, ok := issuer.IssueCell(witness, buffer.Scope(), fixture.output, fixture.rows[row])
	if !ok {
		return binding.Proposal{}, false
	}
	status, ok := model.NewPresence(presence)
	if !ok {
		return binding.Proposal{}, false
	}
	value := binding.ValueToken{}
	if presence == model.Present || presence == model.AuthenticatedOpaque {
		value, ok = issuer.IssueValue(fixture.typeID, testContent("output/value"))
		if !ok {
			return binding.Proposal{}, false
		}
	}
	return binding.NewProposal(destination, value, status)
}

func TestInvokeCallsWorkerExactlyOnceAndReturnsProducedBatch(t *testing.T) {
	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	fixture := newApplyFixture(t, exact, signature.RequirePresent, signature.ProducePresent, 1)
	worker := &scriptedWorker{}
	worker.action = func(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
		proposal, ok := presentProposal(t, fixture, buffer, 0, model.Present)
		if !ok || !buffer.Append(proposal) {
			return refusal(fixture)
		}
		return result(outcome.Produced)
	}
	executor := executorFor(fixture, worker)
	application, ok := executor.Invoke(frameFor(t, fixture, model.Present), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if !ok || !application.Available() || application.Outcome().Code != outcome.Produced || application.Len() != 1 || application.Lineage() != fixture.lineage {
		t.Fatalf("produced application = %#v/%t", application, ok)
	}
	if worker.calls != 1 {
		t.Fatalf("worker calls = %d, want 1", worker.calls)
	}
	batch, ok := application.Proposals()
	if !ok || !batch.Available() {
		t.Fatal("produced batch unavailable")
	}
	proposal, ok := batch.At(0)
	if !ok || !proposal.Available() || proposal.Presence().Kind() != model.Present || proposal.Destination().Column() != fixture.output {
		t.Fatal("produced proposal was not typed and authenticated")
	}
}

func TestInvokeRejectsBadFrameWithoutInvokingWorker(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	fixture := newApplyFixture(t, exact, signature.RequirePresent, signature.ProducePresent, 1)
	worker := &scriptedWorker{action: func(_ binding.Frame, _ *binding.ProposalBuffer) outcome.Result {
		return result(outcome.NoCandidate)
	}}
	application, ok := executorFor(fixture, worker).Invoke(binding.Frame{}, fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if ok || application.Available() {
		t.Fatal("invalid frame was accepted")
	}
	if worker.calls != 0 {
		t.Fatalf("worker calls = %d, want 0", worker.calls)
	}
}

func TestInvokeRejectsUnadmittedLineageBeforeWorker(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	fixture := newApplyFixture(t, exact, signature.RequirePresent, signature.ProducePresent, 1)
	worker := &scriptedWorker{action: func(_ binding.Frame, _ *binding.ProposalBuffer) outcome.Result {
		return result(outcome.NoCandidate)
	}}
	fabricated, ok := model.IssueLineageRef(fixture.lineageAuth.Owner(), testContent("fabricated-lineage"))
	if !ok {
		t.Fatal("fabricated lineage")
	}
	application, ok := executorFor(fixture, worker).Invoke(frameFor(t, fixture, model.Present), fabricated, binding.NewOwnerNamedDestination(fixture.relation))
	if ok || application.Available() {
		t.Fatal("unadmitted lineage was accepted")
	}
	if worker.calls != 0 {
		t.Fatalf("worker calls = %d, want 0", worker.calls)
	}
}

func TestBoundedOverflowReturnsAtomicRefusal(t *testing.T) {
	bounded, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("cardinality")
	}
	fixture := newApplyFixture(t, bounded, signature.RequirePresent, signature.ProducePresent, 3)
	worker := &scriptedWorker{}
	worker.action = func(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
		for index := range fixture.rows {
			proposal, proposalOK := presentProposal(t, fixture, buffer, index, model.Present)
			if !proposalOK || !buffer.Append(proposal) {
				return refusal(fixture)
			}
		}
		return result(outcome.Produced)
	}
	application, ok := executorFor(fixture, worker).Invoke(frameFor(t, fixture, model.Present), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if !ok || !application.Available() || application.Outcome().Code != outcome.Refused {
		t.Fatalf("overflow application = %#v/%t", application, ok)
	}
	if _, proposals := application.Proposals(); proposals {
		t.Fatal("overflow refusal carried a proposal batch")
	}
	if application.Len() != 0 || worker.calls != 1 {
		t.Fatalf("overflow cells/calls = %d/%d", application.Len(), worker.calls)
	}
}

func TestForeignOutputTokenRefusesWithoutPartialCells(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	fixture := newApplyFixture(t, exact, signature.RequirePresent, signature.ProducePresent, 1)
	foreignWitness, ok := fixture.issuer.IssueDenominator(fixture.denominator, mustMembership(t, fixture), testContent("foreign/evidence"))
	if !ok {
		t.Fatal("foreign witness")
	}
	worker := &scriptedWorker{}
	worker.action = func(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
		issuer, issuerOK := binding.NewIssuer(buffer.Fence())
		if !issuerOK {
			return refusal(fixture)
		}
		destination, destinationOK := issuer.IssueCell(foreignWitness, buffer.Scope(), fixture.output, fixture.rows[0])
		value, valueOK := issuer.IssueValue(fixture.typeID, testContent("foreign/value"))
		status, statusOK := model.NewPresence(model.Present)
		proposal, proposalOK := binding.NewProposal(destination, value, status)
		if !destinationOK || !valueOK || !statusOK || !proposalOK {
			return refusal(fixture)
		}
		if buffer.Append(proposal) {
			// A foreign witness must never be accepted by the output gate. If
			// it is, return Produced so the assertion below exposes the leak.
			return result(outcome.Produced)
		}
		return refusal(fixture)
	}
	application, ok := executorFor(fixture, worker).Invoke(frameFor(t, fixture, model.Present), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if !ok || !application.Available() || application.Outcome().Code != outcome.Refused {
		t.Fatalf("foreign output application = %#v/%t", application, ok)
	}
	if _, proposals := application.Proposals(); proposals || application.Len() != 0 {
		t.Fatal("foreign output left partial cells")
	}
}

func TestRefusalHasNoBatchAndAbsenceHasNoBottomValue(t *testing.T) {
	optional, ok := model.NewCardinality(model.Optional, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	fixture := newApplyFixture(t, optional, signature.AllowMissing, signature.ProduceOptional, 1)
	worker := &scriptedWorker{action: func(_ binding.Frame, _ *binding.ProposalBuffer) outcome.Result {
		return refusal(fixture)
	}}
	refused, ok := executorFor(fixture, worker).Invoke(frameFor(t, fixture, model.ProvenAbsent), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if !ok || !refused.Available() || refused.Outcome().Code != outcome.Refused || !refused.Outcome().RefusalID.Available() {
		t.Fatal("refusal outcome was not preserved")
	}
	if _, hasBatch := refused.Proposals(); hasBatch {
		t.Fatal("refusal carried a batch")
	}

	worker = &scriptedWorker{}
	worker.action = func(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
		input, inputOK := frame.At(0)
		cell, cellOK := input.At(0)
		if !inputOK || !cellOK || !cell.Presence().Is(model.ProvenAbsent) {
			return refusal(fixture)
		}
		proposal, proposalOK := presentProposal(t, fixture, buffer, 0, model.ProvenAbsent)
		if !proposalOK || !buffer.Append(proposal) {
			return refusal(fixture)
		}
		return result(outcome.Produced)
	}
	absent, ok := executorFor(fixture, worker).Invoke(frameFor(t, fixture, model.ProvenAbsent), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if !ok || !absent.Available() || absent.Outcome().Code != outcome.Produced || absent.Len() != 1 {
		t.Fatal("proven absence was not published as a distinct status")
	}
	batch, ok := absent.Proposals()
	if !ok {
		t.Fatal("absence batch missing")
	}
	proposal, ok := batch.At(0)
	if !ok || !proposal.Presence().Is(model.ProvenAbsent) || proposal.Value().Available() {
		t.Fatal("absence fabricated a bottom value")
	}
}

func TestNoCandidateNoSelectionAndOpaqueRemainDistinctEmptyBatches(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	fixture := newApplyFixture(t, exact, signature.RequirePresent, signature.ProducePresent, 1)
	for _, code := range []outcome.Code{outcome.NoCandidate, outcome.NoSelection, outcome.Opaque} {
		t.Run(codeName(code), func(t *testing.T) {
			worker := &scriptedWorker{action: func(_ binding.Frame, _ *binding.ProposalBuffer) outcome.Result {
				return result(code)
			}}
			application, ok := executorFor(fixture, worker).Invoke(frameFor(t, fixture, model.Present), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
			if !ok || !application.Available() || application.Outcome().Code != code || application.Len() != 0 {
				t.Fatalf("%s application = %#v/%t", codeName(code), application, ok)
			}
			batch, batchOK := application.Proposals()
			if !batchOK || !batch.Available() || batch.Len() != 0 || batch.Outcome().Code != code {
				t.Fatalf("%s batch = %#v/%t", codeName(code), batch, batchOK)
			}
		})
	}
}

func TestApplicationLeaseSurvivesLaterInvocationWithoutResetAlias(t *testing.T) {
	exact, _ := model.NewCardinality(model.ExactlyOne, 0)
	fixture := newApplyFixture(t, exact, signature.RequirePresent, signature.ProducePresent, 1)
	worker := &scriptedWorker{}
	worker.action = func(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
		proposal, ok := presentProposal(t, fixture, buffer, 0, model.Present)
		if !ok || !buffer.Append(proposal) {
			return refusal(fixture)
		}
		return result(outcome.Produced)
	}
	executor := executorFor(fixture, worker)
	first, ok := executor.Invoke(frameFor(t, fixture, model.Present), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if !ok || !first.Available() {
		t.Fatal("first invocation")
	}
	firstBatch, ok := first.Proposals()
	if !ok || !firstBatch.Available() {
		t.Fatal("first batch")
	}
	second, ok := executor.Invoke(frameFor(t, fixture, model.Present), fixture.lineage, binding.NewOwnerNamedDestination(fixture.relation))
	if !ok || !second.Available() {
		t.Fatal("second invocation")
	}
	if !firstBatch.Available() || firstBatch.Len() != 1 || second.Len() != 1 {
		t.Fatal("one invocation reset another invocation's lease")
	}
	if worker.calls != 2 {
		t.Fatalf("worker calls = %d, want 2", worker.calls)
	}
}

func mustMembership(t *testing.T, fixture applyFixture) binding.MembershipView {
	t.Helper()
	membership, ok := binding.NewMembershipView(fixture.relation, fixture.rows)
	if !ok {
		t.Fatal("membership")
	}
	return membership
}

func codeName(code outcome.Code) string {
	switch code {
	case outcome.NoCandidate:
		return "NoCandidate"
	case outcome.NoSelection:
		return "NoSelection"
	case outcome.Opaque:
		return "Opaque"
	default:
		return "unknown"
	}
}
