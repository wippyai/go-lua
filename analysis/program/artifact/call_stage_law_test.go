package artifact

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
)

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
		input: compiled, occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry),
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
		callID, callOK := compiled.CallIDAt(index)
		callTerm, callTermOK := compiled.Flow().Authored().Calls().At(index)
		row := transaction.calls[index]
		spanID, entryTerm, finishTerm, spanOK := compiled.EvaluationSpan(callTerm)
		entry, entryOK := compiled.Flow().Causal().Sites().ForTerm(entryTerm)
		finish, finishOK := compiled.Flow().Causal().Sites().ForTerm(finishTerm)
		geometry, geometryOK := transaction.occurrenceSpans[occurrenceLookup{kind: OccurrenceCall, id: callID}]
		activation, activationOK := transaction.occurrenceSpans[occurrenceLookup{kind: OccurrenceCallActivation, id: callID}]
		wantEntry, wantFinish := canonicalPoints(transaction.pointIDs(entry)), canonicalPoints(transaction.pointIDs(finish))
		if !callOK || !callTermOK || !spanOK || !entryOK || !finishOK || !geometryOK || !activationOK ||
			!slices.Equal(geometry.entry, wantEntry) || !slices.Equal(geometry.finish, wantFinish) || len(activation.entry) != 0 || !slices.Equal(activation.finish, wantFinish) {
			t.Fatalf("call %d did not preserve exact Entry/Finish geometry", index)
		}
		if !row.Available() || row.ID() != callID || row.SpanID() != spanID {
			t.Fatalf("call %d artifact row did not preserve authored identity and span", index)
		}
		if row.OperandCount() != 2 || row.ArgumentCount() != 1 || row.TypeArgumentCount() != 0 {
			t.Fatalf("call %d child counts operands=%d arguments=%d types=%d, want 2/1/0", index, row.OperandCount(), row.ArgumentCount(), row.TypeArgumentCount())
		}
		for childIndex := 0; childIndex < row.OperandCount(); childIndex++ {
			operand := transaction.callOperands[int(row.operandStart)+childIndex]
			if !operand.Available() || operand.CallID() != row.ID() || !operand.SpanID().Available() {
				t.Fatalf("call %d operand %d is not a closed artifact child", index, childIndex)
			}
		}
		argument := transaction.callArguments[row.argumentStart]
		if !argument.Available() || argument.CallID() != row.ID() || argument.ValuesID() != row.ValuesID() || argument.Index() != 0 {
			t.Fatalf("call %d argument is not joined to its artifact call/value parent", index)
		}
	}
}

func TestProgramArtifactCallStagesUseFinishAndExactDispatchTransport(t *testing.T) {
	entry, finish, callID := valuesLawID(31), valuesLawID(32), valuesLawID(33)
	transaction := compiler{
		points: map[identity.ContentID]struct{}{entry: {}, finish: {}},
		pointGeometry: map[identity.ContentID]Point{
			entry:  {id: entry},
			finish: {id: finish},
		},
		occurrences: []OccurrenceRow{
			{kind: OccurrenceCall, id: callID, points: []identity.ContentID{entry, finish}},
			{kind: OccurrenceCallActivation, id: callID, points: []identity.ContentID{finish}},
		},
		occurrenceSpans: map[occurrenceLookup]occurrenceSpanGeometry{
			{kind: OccurrenceCall, id: callID}:           {entry: []identity.ContentID{entry}, finish: []identity.ContentID{finish}},
			{kind: OccurrenceCallActivation, id: callID}: {finish: []identity.ContentID{finish}},
		},
		localStages: make(map[identity.ContentID]identity.ContentID),
		callStages:  make(map[identity.ContentID]callStageSet),
		events: []WTOEvent{
			{kind: WTOEventPoint, point: entry},
			{kind: WTOEventPoint, point: finish},
		},
	}
	if failure := transaction.deriveRuleOccurrencesFailure(); failure.Available() {
		t.Fatalf("derive call rules: %+v", failure)
	}
	dispatches := transaction.ruleOccurrences[RuleRoleCallDispatch]
	activations := transaction.ruleOccurrences[RuleRoleCallActivation]
	effects := transaction.ruleOccurrences[RuleRoleEffectSelected]
	if len(dispatches) != 1 || len(activations) != 1 || len(effects) != 1 {
		t.Fatalf("distinct Entry/Finish role counts dispatch=%d activation=%d effect=%d, want Finish count 1", len(dispatches), len(activations), len(effects))
	}
	dispatch, activation, effect := dispatches[0], activations[0], effects[0]
	if dispatch.input != finish || dispatch.stage != RuleStageCallDispatch || dispatch.point == entry || dispatch.point == finish {
		t.Fatal("dispatch is not exact Finish -> Dispatch")
	}
	if activation.stage != RuleStageCallSummary || activation.input != dispatch.point || activation.point == activation.input {
		t.Fatal("activation is not exact Dispatch -> Summary")
	}
	if effect.stage != RuleStageCallEffect || effect.input != activation.point || effect.point == effect.input {
		t.Fatal("effect is not exact Summary -> Effect")
	}

	if failure := transaction.installLocalStagesFailure(); failure.Available() {
		t.Fatalf("install call stages: %+v", failure)
	}
	wantDispatchRoles := []RuleRole{RuleRoleValueSource, RuleRolePackSource, RuleRoleHeapIngress, RuleRoleCallDispatch}
	found := false
	for _, transfer := range transaction.localTransfers {
		if transfer.from != finish || transfer.to != dispatch.point {
			continue
		}
		found = true
		if transfer.full || !slices.Equal(transfer.roles, wantDispatchRoles) || slices.Contains(transfer.roles, RuleRoleEffectSelected) {
			t.Fatalf("Base -> Dispatch leaked full environment/Effect or wrong factors: full=%v roles=%v", transfer.full, transfer.roles)
		}
	}
	if !found {
		t.Fatal("Base -> Dispatch transfer missing")
	}
}
