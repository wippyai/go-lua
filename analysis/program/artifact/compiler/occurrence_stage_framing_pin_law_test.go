package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	stageplan "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/stage"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
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
	pinnedCallSummaryEffectTransferFrame   = "analysis/program-artifact/call-summary-effect-transfer"
)

// TestStagedPointIdentityIsThePinnedFramingPreimage pins the staged point
// identities to their framing spelling. Each stage constructor is the digest of
// its pinned framing over the base it is raised from and nothing else.
func TestStagedPointIdentityIsThePinnedFramingPreimage(t *testing.T) {
	base, occurrence := valuesLawID(41), valuesLawID(42)
	left, right, key := valuesLawID(43), valuesLawID(44), schema.Key("value-binary-arithmetic")
	transaction := compiler{
		pointGeometry: map[identity.ContentID]pointDraft{base: {id: base, decisionScope: base}},
		stages:        stageplan.New(artifactFormat()),
		issuance:      transportDirectory(t),
	}
	local, localOK := transaction.localStage(base)
	predecessor, predecessorOK := transaction.predecessorStage(base, key)
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
		{"local stage", local, artifactdigest.Digest(pinnedLocalStageFraming, artifactFormat(), artifactdigest.ContentID(base))},
		{"local predecessor stage", predecessor, artifactdigest.Digest(pinnedLocalPredecessorStageFraming, artifactFormat(), artifactdigest.ContentID(base))},
		{"local computation stage", computation, artifactdigest.Digest(pinnedLocalComputationStageFraming, artifactFormat(), artifactdigest.ContentID(base), artifactdigest.Key(key), artifactdigest.ContentID(occurrence))},
		{"call dispatch stage", stages.Dispatch(), artifactdigest.Digest(pinnedCallDispatchStageFraming, artifactFormat(), artifactdigest.ContentID(base))},
		{"call summary stage", stages.Summary(), artifactdigest.Digest(pinnedCallSummaryStageFraming, artifactFormat(), artifactdigest.ContentID(base))},
		{"call effect stage", stages.Effect(), artifactdigest.Digest(pinnedCallEffectStageFraming, artifactFormat(), artifactdigest.ContentID(base))},
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
		pointGeometry: map[identity.ContentID]pointDraft{
			entry:  {id: entry, decisionScope: entry},
			finish: {id: finish, decisionScope: finish},
		},
		occurrenceSpans: map[occurrenceLookup]occurrenceSpanGeometry{
			{kind: programschema.OccurrenceCall, id: callID}: {entry: []identity.ContentID{entry}, finish: []identity.ContentID{finish}},
		},
		stages: stageplan.New(artifactFormat()),
		events: []wtoEventDraft{
			{kind: wtoEventPoint, point: entry},
			{kind: wtoEventPoint, point: finish},
		},
		issuance: transportDirectory(t, []issuance.Placement{
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallDispatch, Key: "call-dispatch", Writes: "call", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallSummary, Key: "call-summary", Writes: "call", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormBase, Input: programschema.RuleInputNone, Stage: programschema.RuleStageBase, Key: "pack-source", Writes: "pack", Transport: true},
			{Occurrence: programschema.OccurrenceCall, Requirement: issuance.RequirementUnrestricted, Form: issuance.FormCallStage, Input: programschema.RuleInputFinish, Stage: programschema.RuleStageCallEffect, Key: "effect-selected", Writes: "effect", Transport: true},
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
	dispatch := artifactdigest.Digest(pinnedCallDispatchStageFraming, artifactFormat(), artifactdigest.ContentID(finish))
	summary := artifactdigest.Digest(pinnedCallSummaryStageFraming, artifactFormat(), artifactdigest.ContentID(finish))
	effect := artifactdigest.Digest(pinnedCallEffectStageFraming, artifactFormat(), artifactdigest.ContentID(finish))
	for _, staged := range []identity.ContentID{dispatch, summary, effect} {
		row, installed := transaction.pointGeometry[staged]
		if !installed {
			t.Fatalf("the installation pass carries no point at the pinned staged identity %v", staged)
		}
		if row.decisionScope != finish || len(row.decisions) != 0 {
			t.Fatalf("staged point %v did not reference finish scope without copying decisions: scope=%v decisions=%v", staged, row.decisionScope, row.decisions)
		}
	}
	framings := map[[2]identity.ContentID]string{
		{finish, dispatch}:  pinnedCallBaseDispatchTransferFraming,
		{finish, summary}:   pinnedCallBaseSummaryTransferFraming,
		{finish, effect}:    pinnedCallBaseEffectTransferFraming,
		{dispatch, summary}: pinnedCallDispatchSummaryTransferFrame,
		{dispatch, effect}:  pinnedCallDispatchEffectTransferFrame,
		{summary, effect}:   pinnedCallSummaryEffectTransferFrame,
	}
	if fault := transaction.localTransfer.Seal(); fault.Available() {
		t.Fatalf("seal local transfers: %#v", fault)
	}
	transfers, transferWrites, transferFault := transaction.localTransfer.TakeCanonicalPlanes()
	if transferFault.Available() || len(transfers) != len(framings) {
		t.Fatalf("the call splice installed %d transports, want %d (fault=%+v)", len(transfers), len(framings), transferFault)
	}
	for _, edge := range transfers {
		from, to, id := edge.From(), edge.To(), edge.ID()
		framing, pinned := framings[[2]identity.ContentID{from, to}]
		if !pinned {
			t.Fatalf("the call splice installed an unpinned transport %v -> %v", from, to)
		}
		offset, count, spanOK := edge.WriteSpan()
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
		fields := []artifactdigest.Field{artifactdigest.ContentID(from), artifactdigest.ContentID(to), artifactdigest.Bool(edge.Full()), artifactdigest.Uint(uint64(len(writes)))}
		for _, write := range writes {
			fields = append(fields, artifactdigest.Key(write))
		}
		if !from.Available() || !to.Available() || !id.Available() || !spanOK {
			t.Fatalf("transport row unavailable: from=%t to=%t id=%t span=%t", from.Available(), to.Available(), id.Available(), spanOK)
		}
		if want := artifactdigest.Digest(framing, artifactFormat(), fields...); id != want {
			t.Fatalf("transport %v -> %v identity = %v, the pinned framing preimage is %v", from, to, id, want)
		}
	}
}
