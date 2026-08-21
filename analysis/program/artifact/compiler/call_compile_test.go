package compiler

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	stageplan "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/stage"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestCallConstructionRejectsMissingCompileKey(t *testing.T) {
	compiled, err := lower.Lower(lower.Source{Name: "artifact-missing-compile-key.lua", Text: []byte(`
local function identity(value)
  return value
end
return identity(true)
`)})
	if err != nil {
		t.Fatal(err)
	}
	transaction := compiler{input: compiled, pointIDsBySite: make(map[identity.ContentID][]identity.ContentID)}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %+v", failure)
	}
	if _, ok := transaction.callConstruction(0); ok {
		t.Fatal("call construction compensated for a missing canonical CompileKey")
	}
}

func TestProgramArtifactCopyCallsRecordsExactSpan(t *testing.T) {
	compiled, err := lower.Lower(lower.Source{Name: "artifact-call-span.lua", Text: []byte(`
local function identity(value)
  return value
end
return identity(true)
`)})
	if err != nil {
		t.Fatal(err)
	}
	transaction := compiler{
		input: compiled, key: testCompileKey(t, compiled), occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry),
		pointIDsBySite: make(map[identity.ContentID][]identity.ContentID),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %+v", failure)
	}
	if failure := transaction.copyCalls(); failure.Available() {
		t.Fatalf("copy calls: %+v", failure)
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %+v", failure)
	}
	if failure := transaction.copyCallRowsFailure(); failure.Available() {
		t.Fatalf("copy call rows: %+v", failure)
	}
	callCount := compiled.Flow().Authored().Calls().Count()
	if callCount == 0 {
		t.Fatal("fixture did not issue a call")
	}
	if len(transaction.calls) != callCount {
		t.Fatalf("artifact call rows = %d, want authored denominator %d", len(transaction.calls), callCount)
	}
	for index := 0; index < callCount; index++ {
		callID, callOK := testCallIdentityAt(compiled, index)
		callTerm, callTermOK := compiled.Flow().Authored().Calls().At(index)
		row := transaction.calls[index]
		spanID, entryTerm, finishTerm, spanOK := compiled.EvaluationSpan(callTerm)
		entry, entryOK := compiled.Flow().Causal().Sites().ForTerm(entryTerm)
		finish, finishOK := compiled.Flow().Causal().Sites().ForTerm(finishTerm)
		geometry, geometryOK := transaction.occurrenceSpans[occurrenceLookup{kind: programschema.OccurrenceCall, id: callID}]
		activation, activationOK := transaction.occurrenceSpans[occurrenceLookup{kind: programschema.OccurrenceCallActivation, id: callID}]
		wantEntry, wantFinish := canonicalPoints(transaction.pointIDs(entry)), canonicalPoints(transaction.pointIDs(finish))
		if !callOK || !callTermOK || !spanOK || !entryOK || !finishOK || !geometryOK || !activationOK ||
			!slices.Equal(geometry.entry, wantEntry) || !slices.Equal(geometry.finish, wantFinish) ||
			len(activation.entry) != 0 || !slices.Equal(activation.finish, wantFinish) {
			t.Fatalf("call %d did not preserve exact Entry/Finish geometry", index)
		}
		if !row.Available() || row.ID() != callID || row.SpanID() != spanID {
			t.Fatalf("call %d artifact row did not preserve authored identity and span", index)
		}
		direct, directOK := compiled.Flow().DirectFunctions().Call(callTerm)
		target, targetOK := row.DirectTargetBody()
		if directOK != targetOK {
			t.Fatalf("call %d direct-target join = %v, row %v", index, directOK, targetOK)
		}
		if directOK {
			boundary, boundaryOK := compiled.Flow().FunctionBoundaries().For(direct)
			bodyTerm, bodyOK := boundary.Body()
			bodyPath, pathOK := compiled.Flow().BodyPath(bodyTerm)
			if !boundaryOK || !bodyOK || !pathOK || target != bodyPath {
				t.Fatalf("call %d target body %x does not match DirectFunctions", index, target[:4])
			}
		}
		if row.OperandCount() != 2 || row.ArgumentCount() != 1 || row.TypeArgumentCount() != 0 {
			t.Fatalf("call %d child counts operands=%d arguments=%d types=%d, want 2/1/0", index, row.OperandCount(), row.ArgumentCount(), row.TypeArgumentCount())
		}
		operandOffset, operandWidth, operandSpanOK := row.OperandSpan()
		argumentOffset, argumentWidth, argumentSpanOK := row.ArgumentSpan()
		if !operandSpanOK || !argumentSpanOK || int(operandWidth) != row.OperandCount() || int(argumentWidth) != row.ArgumentCount() {
			t.Fatalf("call %d published invalid child spans", index)
		}
		for childIndex := uint32(0); childIndex < operandWidth; childIndex++ {
			operand := transaction.callOperands[int(operandOffset+childIndex)]
			if !operand.Available() || operand.CallID() != row.ID() || !operand.SpanID().Available() {
				t.Fatalf("call %d operand %d is not a closed artifact child", index, childIndex)
			}
		}
		argument := transaction.callArguments[argumentOffset]
		if !argument.Available() || argument.CallID() != row.ID() || argument.ValuesID() != row.ValuesID() || argument.Index() != 0 {
			t.Fatalf("call %d argument is not joined to its artifact call/value parent", index)
		}
	}
}

func TestBoundedTailCallResultUsesDistinctValueBeforeStorageCell(t *testing.T) {
	compiled, err := lower.Lower(lower.Source{Name: "artifact-call-result-storage.lua", Text: []byte(`
local result = require("module")
return result
`)})
	if err != nil {
		t.Fatal(err)
	}
	transaction := compiler{
		input: compiled, key: testCompileKey(t, compiled), occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry),
		pointIDsBySite: make(map[identity.ContentID][]identity.ContentID),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %+v", failure)
	}
	if failure := transaction.copyCalls(); failure.Available() {
		t.Fatalf("copy calls: %+v", failure)
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %+v", failure)
	}
	if failure := transaction.copyCallRowsFailure(); failure.Available() {
		t.Fatalf("copy call rows: %+v", failure)
	}
	var tail programschema.CallResultSlot
	for _, slot := range transaction.callResultSlots {
		if slot.SourceKind() == programschema.CallResultSlotSourceValuesTail && slot.ConsumerKind() == programschema.CallResultSlotConsumerCell {
			tail = slot
			break
		}
	}
	valueID, valueOK := tail.ValueID()
	cellID := tail.ConsumerID()
	if !tail.Available() || !valueOK || valueID == cellID {
		t.Fatalf("tail slot did not separate producer Value %x from consumer Cell %x", valueID[:4], cellID[:4])
	}
	foundTransfer := false
	for index := 0; index < compiled.Flow().Authored().Storage().Binds().Count(); index++ {
		bind, bindOK := transaction.storageBindAt(index)
		if !bindOK {
			continue
		}
		for _, transfer := range bind.transfers {
			if transfer.value == valueID && transfer.cell == cellID {
				foundTransfer = true
			}
		}
	}
	if !foundTransfer {
		t.Fatal("bounded tail slot published no explicit Value-to-Cell storage transfer")
	}
}

func testCallIdentityAt(input *program.Program, index int) (identity.ContentID, bool) {
	if input == nil || index < 0 {
		return identity.ContentID{}, false
	}
	flowView := input.Flow()
	calls := flowView.Authored().Calls()
	call, callOK := calls.At(index)
	owner, callee, receiver, actuals, rowOK := calls.Get(call)
	bodyBoundary, bodyOK := flowView.FunctionBoundaries().ForBody(owner)
	spanID, _, _, spanOK := input.EvaluationSpan(call)
	valuesID, valuesOK := flowView.ValuesOccurrenceID(actuals)
	contracts := input.Static().Contracts().Calls()
	typeCount, typeCountOK := contracts.TypeArgumentCount(call)
	typeArguments, typeArgumentsOK := contracts.TypeArgumentID(call)
	form := programschema.CallFormPlain
	if receiver != 0 {
		form = programschema.CallFormMethod
	}
	if !callOK || !rowOK || !bodyOK || !bodyBoundary.Available() || !spanOK || !spanID.Available() || !valuesOK ||
		!valuesID.Available() || !typeCountOK || typeCount < 0 || !typeArgumentsOK || !typeArguments.Available() {
		return identity.ContentID{}, false
	}
	identities, identitiesOK := programschema.CallIdentities(programschema.CallIdentityInput{
		ProgramID: input.ContentID(), Call: call, Form: form, Body: bodyBoundary.ContextID(), Span: spanID,
		Callee: callee, Receiver: receiver, Actuals: actuals, Values: valuesID,
		TypeArgumentCount: typeCount, TypeArguments: typeArguments,
	})
	return identities.Call, identitiesOK && identities.Call.Available()
}

func TestProgramArtifactCallStagesUseFinishAndExactDispatchTransport(t *testing.T) {
	entry, finish, callID := valuesLawID(31), valuesLawID(32), valuesLawID(33)
	transaction := compiler{
		pointGeometry: map[identity.ContentID]pointDraft{
			entry:  {id: entry, decisionScope: entry},
			finish: {id: finish, decisionScope: finish},
		},
		occurrenceSpans: map[occurrenceLookup]occurrenceSpanGeometry{
			{kind: programschema.OccurrenceCall, id: callID}:           {entry: []identity.ContentID{entry}, finish: []identity.ContentID{finish}},
			{kind: programschema.OccurrenceCallActivation, id: callID}: {finish: []identity.ContentID{finish}},
		},
		stages: stageplan.New(artifactFormat()),
		events: []wtoEventDraft{
			{kind: wtoEventPoint, point: entry},
			{kind: wtoEventPoint, point: finish},
		},
		issuance: transportDirectory(t, []issuance.Placement{
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallDispatch, Key: "call-dispatch", Writes: "call", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormBase, Input: programschema.RuleInputNone, Stage: programschema.RuleStageBase, Key: "pack-source", Writes: "pack", Transport: true},
			{Occurrence: programschema.OccurrenceValueSource, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormBase, Input: programschema.RuleInputNone, Stage: programschema.RuleStageBase, Key: "value-source", Writes: "value", Transport: true},
			{Occurrence: programschema.OccurrenceAllocation, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormBase, Input: programschema.RuleInputNone, Stage: programschema.RuleStageBase, Key: "heap-ingress", Writes: "heap", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallEffect, Key: "effect-selected", Writes: "effect", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallEffect, Key: "effect-opaque", Writes: "effect", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallEffect, Key: "effect-body", Writes: "effect", Transport: true},
			{Occurrence: programschema.OccurrenceCallActivation, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallSummary, Key: "call-activation", Writes: "call"},
		}...),
	}
	if !transaction.appendOccurrence(programschema.OccurrenceCall, callID, identity.ContentID{}, []identity.ContentID{entry, finish}, nil, 0) ||
		!transaction.appendOccurrence(programschema.OccurrenceCallActivation, callID, identity.ContentID{}, []identity.ContentID{finish}, nil, 0) {
		t.Fatal("failed to append canonical call occurrence fixture")
	}
	if failure := transaction.deriveRuleOccurrencesFailure(); failure.Available() {
		t.Fatalf("derive call rules: %+v", failure)
	}
	var dispatch, activation, effect programschema.RuleOccurrence
	var dispatchCount, activationCount, effectCount int
	for _, placement := range transaction.ruleOccurrences {
		switch placement.Key() {
		case "call-dispatch":
			dispatch, dispatchCount = placement, dispatchCount+1
		case "call-activation":
			activation, activationCount = placement, activationCount+1
		case "effect-selected":
			effect, effectCount = placement, effectCount+1
		}
	}
	if dispatchCount != 1 || activationCount != 1 || effectCount != 1 {
		t.Fatalf("distinct Entry/Finish key counts dispatch=%d activation=%d effect=%d, want Finish count 1", dispatchCount, activationCount, effectCount)
	}
	dispatchInput, dispatchInputOK := dispatch.InputPoint()
	activationInput, activationInputOK := activation.InputPoint()
	effectInput, effectInputOK := effect.InputPoint()
	if !dispatchInputOK || dispatchInput != finish || dispatch.Stage() != programschema.RuleStageCallDispatch || dispatch.PointID() == entry || dispatch.PointID() == finish {
		t.Fatal("dispatch is not exact Finish -> Dispatch")
	}
	if !activationInputOK || activation.Stage() != programschema.RuleStageCallSummary || activationInput != dispatch.PointID() || activation.PointID() == activationInput {
		t.Fatal("activation is not exact Dispatch -> Summary")
	}
	if !effectInputOK || effect.Stage() != programschema.RuleStageCallEffect || effectInput != activation.PointID() || effect.PointID() == effectInput {
		t.Fatal("effect is not exact Summary -> Effect")
	}

	if failure := transaction.installLocalStagesFailure(); failure.Available() {
		t.Fatalf("install call stages: %+v", failure)
	}
	if fault := transaction.localTransfer.Seal(); fault.Failed() {
		t.Fatalf("seal local transfers: %#v", fault)
	}
	transfers, transferWrites, transfersOK := transaction.localTransfer.TakeCanonicalPlanes()
	if !transfersOK {
		t.Fatal("take local transfer planes")
	}
	wantDispatchWrites := []schema.Key{"effect-body", "heap-ingress", "pack-source", "value-source"}
	found := false
	for _, transfer := range transfers {
		from, to := transfer.From(), transfer.To()
		if !from.Available() || !to.Available() || from != finish || to != dispatch.PointID() {
			continue
		}
		found = true
		offset, count, spanOK := transfer.WriteSpan()
		writes := make([]schema.Key, 0, count)
		if spanOK && uint64(offset)+uint64(count) <= uint64(len(transferWrites)) {
			for index := uint32(0); index < count; index++ {
				write, writeOK := transferWrites[offset+index].Key()
				if !writeOK {
					spanOK = false
					break
				}
				writes = append(writes, write)
			}
		}
		if !spanOK || transfer.Full() || !slices.Equal(writes, wantDispatchWrites) || slices.Contains(writes, "effect-selected") {
			t.Fatalf("Base -> Dispatch leaked full environment/Effect or wrong factors: full=%v writes=%v", transfer.Full(), writes)
		}
	}
	if !found {
		t.Fatal("Base -> Dispatch transfer missing")
	}
}
