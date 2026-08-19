package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// The framing strings below are digest preimages. A staged point identity and a
// stage transport identity are derived from the literal spelling, so the
// spelling is part of every compiled artifact's content address: moving a
// framing into a declaration must carry the exact bytes, or every staged point
// and every stage transport in every artifact takes a new identity.
const (
	pinnedLocalStageFraming            = "analysis/program-artifact/local-stage"
	pinnedLocalPredecessorStageFraming = "analysis/program-artifact/local-predecessor-stage"
	pinnedLocalComputationStageFraming = "analysis/program-artifact/local-computation-stage"
	pinnedCallDispatchStageFraming     = "analysis/program-artifact/call-dispatch-stage"
	pinnedCallSummaryStageFraming      = "analysis/program-artifact/call-summary-stage"
	pinnedCallEffectStageFraming       = "analysis/program-artifact/call-effect-stage"

	pinnedLocalTransferFraming             = "analysis/program-artifact/local-transfer"
	pinnedLocalComputationTransferFraming  = "analysis/program-artifact/local-computation-transfer"
	pinnedCallBaseDispatchTransferFraming  = "analysis/program-artifact/call-base-dispatch-transfer"
	pinnedCallBaseSummaryTransferFraming   = "analysis/program-artifact/call-base-summary-transfer"
	pinnedCallBaseEffectTransferFraming    = "analysis/program-artifact/call-base-effect-transfer"
	pinnedCallDispatchSummaryTransferFrame = "analysis/program-artifact/call-dispatch-summary-transfer"
	pinnedCallDispatchEffectTransferFrame  = "analysis/program-artifact/call-dispatch-effect-transfer"
)

// TestStagedPointIdentityIsThePinnedFramingPreimage pins the staged point
// identities to their framing spelling. Each stage constructor is the digest of
// its pinned framing over the base it is raised from and nothing else.
func TestStagedPointIdentityIsThePinnedFramingPreimage(t *testing.T) {
	base, occurrence := valuesLawID(41), valuesLawID(42)
	left, right, key := valuesLawID(43), valuesLawID(44), schema.Key("value-binary-arithmetic")
	transaction := compiler{
		pointGeometry:     map[identity.ContentID]Point{base: {id: base}},
		localStages:       make(map[identity.ContentID]identity.ContentID),
		predecessorStages: make(map[identity.ContentID]identity.ContentID),
		computationStages: make(map[identity.ContentID][]computationStage),
		callStages:        make(map[identity.ContentID]callStageSet),
		issuance:          transportDirectory(t),
	}
	local, localOK := transaction.localStage(base)
	predecessor, predecessorOK := transaction.predecessorStage(base)
	computation, computationOK := transaction.localComputationStage(base, key, occurrence, left, right)
	stages, stagesOK := transaction.callStage(base)
	if !localOK || !predecessorOK || !computationOK || !stagesOK {
		t.Fatal("the stage constructors raised no staged point over an available base")
	}
	pinned := []struct {
		label string
		got   identity.ContentID
		want  identity.ContentID
	}{
		{"local stage", local, digest(pinnedLocalStageFraming, artifactFormat, bytesField(base))},
		{"local predecessor stage", predecessor, digest(pinnedLocalPredecessorStageFraming, artifactFormat, bytesField(base))},
		{"local computation stage", computation, digest(pinnedLocalComputationStageFraming, artifactFormat, bytesField(base), keyField(key), bytesField(occurrence))},
		{"call dispatch stage", stages.dispatch, digest(pinnedCallDispatchStageFraming, artifactFormat, bytesField(base))},
		{"call summary stage", stages.summary, digest(pinnedCallSummaryStageFraming, artifactFormat, bytesField(base))},
		{"call effect stage", stages.effect, digest(pinnedCallEffectStageFraming, artifactFormat, bytesField(base))},
	}
	seen := make(map[identity.ContentID]string, len(pinned))
	for _, row := range pinned {
		if !row.got.Available() || row.got != row.want {
			t.Fatalf("%s identity = %v, the pinned framing preimage is %v", row.label, row.got, row.want)
		}
		if prior, duplicate := seen[row.got]; duplicate {
			t.Fatalf("%s and %s share one staged point identity", prior, row.label)
		}
		seen[row.got] = row.label
	}
}

// TestInstalledCallStageIdentitiesArePinnedOverAFixture carries the pin through
// the installation pass: the points and transports a compiled call splice
// carries are the pinned framing preimages, not identities the pass invents.
func TestInstalledCallStageIdentitiesArePinnedOverAFixture(t *testing.T) {
	entry, finish, callID := valuesLawID(51), valuesLawID(52), valuesLawID(53)
	transaction := compiler{
		pointGeometry: map[identity.ContentID]Point{
			entry:  {id: entry},
			finish: {id: finish},
		},
		occurrenceSpans: map[occurrenceLookup]occurrenceSpanGeometry{
			{kind: programschema.OccurrenceCall, id: callID}: {entry: []identity.ContentID{entry}, finish: []identity.ContentID{finish}},
		},
		localStages: make(map[identity.ContentID]identity.ContentID),
		callStages:  make(map[identity.ContentID]callStageSet),
		events: []WTOEvent{
			{kind: WTOEventPoint, point: entry},
			{kind: WTOEventPoint, point: finish},
		},
		issuance: transportDirectory(t, []IssuancePlacement{
			{Occurrence: programschema.OccurrenceCall, Requirement: IssuanceRequirementUnrestricted, Form: IssuanceFormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallDispatch, Key: "call-dispatch", Writes: "call", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: IssuanceRequirementUnrestricted, Form: IssuanceFormBase, Input: programschema.RuleInputNone, Stage: programschema.RuleStageBase, Key: "pack-source", Writes: "pack", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: IssuanceRequirementUnrestricted, Form: IssuanceFormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallEffect, Key: "effect-selected", Writes: "effect", Transport: true},
		}...),
	}
	if !transaction.appendOccurrence(programschema.OccurrenceCall, callID, identity.ContentID{}, []identity.ContentID{entry, finish}, nil, 0) {
		t.Fatal("failed to append canonical call occurrence fixture")
	}
	if failure := transaction.deriveRuleOccurrencesFailure(); failure.Available() {
		t.Fatalf("derive call rules: %+v", failure)
	}
	if failure := transaction.installLocalStagesFailure(); failure.Available() {
		t.Fatalf("install call stages: %+v", failure)
	}
	dispatch := digest(pinnedCallDispatchStageFraming, artifactFormat, bytesField(finish))
	summary := digest(pinnedCallSummaryStageFraming, artifactFormat, bytesField(finish))
	effect := digest(pinnedCallEffectStageFraming, artifactFormat, bytesField(finish))
	for _, staged := range []identity.ContentID{dispatch, summary, effect} {
		if _, installed := transaction.pointGeometry[staged]; !installed {
			t.Fatalf("the installation pass carries no point at the pinned staged identity %v", staged)
		}
	}
	framings := map[[2]identity.ContentID]string{
		{finish, dispatch}:  pinnedCallBaseDispatchTransferFraming,
		{finish, summary}:   pinnedCallBaseSummaryTransferFraming,
		{finish, effect}:    pinnedCallBaseEffectTransferFraming,
		{dispatch, summary}: pinnedCallDispatchSummaryTransferFrame,
		{dispatch, effect}:  pinnedCallDispatchEffectTransferFrame,
	}
	if len(transaction.localTransfers) != len(framings) {
		t.Fatalf("the call splice installed %d transports, want %d", len(transaction.localTransfers), len(framings))
	}
	for _, edge := range transaction.localTransfers {
		framing, pinned := framings[[2]identity.ContentID{edge.from, edge.to}]
		if !pinned {
			t.Fatalf("the call splice installed an unpinned transport %v -> %v", edge.from, edge.to)
		}
		fields := []field{bytesField(edge.from), bytesField(edge.to), boolField(edge.full), uintField(uint64(len(edge.writes)))}
		for _, write := range edge.writes {
			fields = append(fields, keyField(write))
		}
		if want := digest(framing, artifactFormat, fields...); edge.id != want {
			t.Fatalf("transport %v -> %v identity = %v, the pinned framing preimage is %v", edge.from, edge.to, edge.id, want)
		}
	}
}
