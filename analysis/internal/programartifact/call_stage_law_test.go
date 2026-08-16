package programartifact

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower"
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
		input: compiled.TransformerInput(), occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry),
		pointIDsBySite: make(map[keyspace.ContentID][]keyspace.ContentID),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %+v", failure)
	}
	if failure := transaction.copyCalls(); failure.Available() {
		t.Fatalf("copy calls: %+v", failure)
	}
	if transaction.input.CallCount() == 0 {
		t.Fatal("fixture did not issue a call")
	}
	for index := 0; index < transaction.input.CallCount(); index++ {
		call, callOK := transaction.input.CallAt(index)
		span, spanOK := call.Span()
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		geometry, geometryOK := transaction.occurrenceSpans[occurrenceLookup{kind: OccurrenceCall, id: call.ContextID()}]
		activation, activationOK := transaction.occurrenceSpans[occurrenceLookup{kind: OccurrenceCallActivation, id: call.ContextID()}]
		wantEntry, wantFinish := canonicalPoints(transaction.pointIDs(entry)), canonicalPoints(transaction.pointIDs(finish))
		if !callOK || !spanOK || !entryOK || !finishOK || !geometryOK || !activationOK ||
			!slices.Equal(geometry.entry, wantEntry) || !slices.Equal(geometry.finish, wantFinish) || len(activation.entry) != 0 || !slices.Equal(activation.finish, wantFinish) {
			t.Fatalf("call %d did not preserve exact Entry/Finish geometry", index)
		}
	}
}

func TestProgramArtifactCallStagesUseFinishAndExactDispatchTransport(t *testing.T) {
	entry, finish, callID := valuesLawID(31), valuesLawID(32), valuesLawID(33)
	transaction := compiler{
		points: map[keyspace.ContentID]struct{}{entry: {}, finish: {}},
		pointGeometry: map[keyspace.ContentID]Point{
			entry:  {id: entry},
			finish: {id: finish},
		},
		occurrences: []OccurrenceRow{
			{kind: OccurrenceCall, id: callID, points: []keyspace.ContentID{entry, finish}},
			{kind: OccurrenceCallActivation, id: callID, points: []keyspace.ContentID{finish}},
		},
		occurrenceSpans: map[occurrenceLookup]occurrenceSpanGeometry{
			{kind: OccurrenceCall, id: callID}:           {entry: []keyspace.ContentID{entry}, finish: []keyspace.ContentID{finish}},
			{kind: OccurrenceCallActivation, id: callID}: {finish: []keyspace.ContentID{finish}},
		},
		localStages: make(map[keyspace.ContentID]keyspace.ContentID),
		callStages:  make(map[keyspace.ContentID]callStageSet),
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
