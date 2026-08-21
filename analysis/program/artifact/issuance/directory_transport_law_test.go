package issuance

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

const (
	localFraming            = "analysis/program-artifact/local-stage"
	localPredecessorFraming = "analysis/program-artifact/local-predecessor-stage"
	localComputationFraming = "analysis/program-artifact/local-computation-stage"
	callDispatchFraming     = "analysis/program-artifact/call-dispatch-stage"
	callSummaryFraming      = "analysis/program-artifact/call-summary-stage"
	callEffectFraming       = "analysis/program-artifact/call-effect-stage"
)

type transportDeclaration struct {
	key       schema.Key
	writes    schema.Key
	stage     programschema.RuleStage
	transport bool
}

func declaredStageFramings() (map[Form]string, map[programschema.RuleStage]string) {
	return map[Form]string{
			FormLocal:            localFraming,
			FormComputation:      localComputationFraming,
			FormLocalPredecessor: localPredecessorFraming,
			FormLocalSuccessor:   "analysis/program-artifact/local-successor-stage",
		}, map[programschema.RuleStage]string{
			programschema.RuleStageCallDispatch: callDispatchFraming,
			programschema.RuleStageCallSummary:  callSummaryFraming,
			programschema.RuleStageCallEffect:   callEffectFraming,
		}
}

func transportPlacements(declarations ...transportDeclaration) []Placement {
	placements := make([]Placement, 0, len(declarations))
	for _, declaration := range declarations {
		placement := Placement{Occurrence: programschema.OccurrenceCall, Form: FormBase, Input: programschema.RuleInputNone, Stage: declaration.stage, Requirement: RequirementUnrestricted, Key: declaration.key, Writes: declaration.writes, Transport: declaration.transport}
		if declaration.stage != programschema.RuleStageBase {
			placement.Form, placement.Input = FormCallStage, programschema.RuleInputFinish
		}
		placements = append(placements, placement)
	}
	return placements
}

func transportDirectory(t *testing.T, placements ...Placement) Directory {
	t.Helper()
	forms, stages := declaredStageFramings()
	directory, ok := NewDirectory(placements, forms, stages)
	if !ok {
		t.Fatal("the declared placements and framings were refused admission")
	}
	return directory
}

func declaredCallStagePlacements() []Placement {
	return transportPlacements(
		transportDeclaration{key: "value-source", writes: "value", stage: programschema.RuleStageBase, transport: true},
		transportDeclaration{key: "raw-get", writes: "value", stage: programschema.RuleStageLocal, transport: true},
		transportDeclaration{key: "pack-source", writes: "pack", stage: programschema.RuleStageBase, transport: true},
		transportDeclaration{key: "heap-ingress", writes: "heap", stage: programschema.RuleStageBase, transport: true},
		transportDeclaration{key: "call-dispatch", writes: "call", stage: programschema.RuleStageCallDispatch, transport: true},
		transportDeclaration{key: "call-activation", writes: "call", stage: programschema.RuleStageCallSummary, transport: false},
		transportDeclaration{key: "value-result", writes: "value", stage: programschema.RuleStageCallSummary, transport: true},
		transportDeclaration{key: "effect-selected", writes: "effect", stage: programschema.RuleStageCallEffect, transport: true},
		transportDeclaration{key: "effect-body", writes: "effect", stage: programschema.RuleStageCallEffect, transport: true},
	)
}

func assertKeys(t *testing.T, label string, got []schema.Key, want ...schema.Key) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for index, key := range want {
		if got[index] != key {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}

func TestOrderedKeysCanonicalizesWithoutAliasingInput(t *testing.T) {
	input := []schema.Key{"value-source", "pack-source", "heap-ingress", "call-dispatch"}
	ordered, ok := OrderedKeys(input)
	if !ok {
		t.Fatal("available distinct keys were refused")
	}
	assertKeys(t, "ordered keys", ordered, "call-dispatch", "heap-ingress", "pack-source", "value-source")
	if !slices.Equal(input, []schema.Key{"value-source", "pack-source", "heap-ingress", "call-dispatch"}) {
		t.Fatalf("ordering mutated input: %v", input)
	}
	ordered[0] = "changed"
	if input[0] != "value-source" {
		t.Fatal("ordered keys aliases input")
	}
	if _, ok := OrderedKeys([]schema.Key{"value-source", "value-source"}); ok {
		t.Fatal("duplicate key was accepted")
	}
}

func TestCallStageTransportIsDerivedFromDeclaredWrites(t *testing.T) {
	dispatch, effectEntry, bypass, forward, summary, ok := transportDirectory(t, declaredCallStagePlacements()...).CallStageTransport()
	if !ok {
		t.Fatal("the declared directory produced no call-stage transport plan")
	}
	assertKeys(t, "dispatch entry", dispatch, "effect-body", "heap-ingress", "pack-source", "raw-get")
	assertKeys(t, "effect entry", effectEntry, "call-dispatch", "heap-ingress", "pack-source", "raw-get")
	assertKeys(t, "effect bypass", bypass, "effect-body")
	assertKeys(t, "dispatch forward", forward, "call-dispatch")
	assertKeys(t, "summary forward", summary, "call-dispatch", "effect-body", "raw-get")
}

func TestCallStageTransportFollowsAnAddedDeclaredAxis(t *testing.T) {
	extended := append(declaredCallStagePlacements(), transportPlacements(transportDeclaration{key: "probe-source", writes: "probe", stage: programschema.RuleStageBase, transport: true})...)
	dispatch, effectEntry, bypass, forward, summary, ok := transportDirectory(t, extended...).CallStageTransport()
	if !ok {
		t.Fatal("the extended directory produced no call-stage transport plan")
	}
	assertKeys(t, "dispatch entry", dispatch, "effect-body", "heap-ingress", "pack-source", "probe-source", "raw-get")
	assertKeys(t, "effect entry", effectEntry, "call-dispatch", "heap-ingress", "pack-source", "probe-source", "raw-get")
	assertKeys(t, "effect bypass", bypass, "effect-body")
	assertKeys(t, "dispatch forward", forward, "call-dispatch")
	assertKeys(t, "summary forward", summary, "call-dispatch", "effect-body", "raw-get")
}

func TestCallEffectWriterStillReceivesItsBaseAxisThroughSummary(t *testing.T) {
	extended := append(declaredCallStagePlacements(), transportPlacements(
		transportDeclaration{key: "value-effect", writes: "value", stage: programschema.RuleStageCallEffect, transport: true},
	)...)
	dispatch, effectEntry, bypass, _, summary, ok := transportDirectory(t, extended...).CallStageTransport()
	if !ok {
		t.Fatal("the value-effect directory produced no call-stage transport plan")
	}
	assertKeys(t, "value-effect dispatch entry", dispatch, "effect-body", "heap-ingress", "pack-source", "raw-get")
	assertKeys(t, "value-effect effect entry", effectEntry, "call-dispatch", "heap-ingress", "pack-source")
	assertKeys(t, "value-effect summary bypass", bypass, "effect-body", "raw-get")
	assertKeys(t, "value-effect summary forward", summary, "call-dispatch", "effect-body", "raw-get")
}

func TestCallStageTransportRefusesAStageWithNoDeclaredWrite(t *testing.T) {
	for _, withdrawn := range []programschema.RuleStage{programschema.RuleStageCallDispatch, programschema.RuleStageCallSummary, programschema.RuleStageCallEffect} {
		declared := declaredCallStagePlacements()
		remaining := make([]Placement, 0, len(declared))
		for _, placement := range declared {
			if placement.Stage != withdrawn {
				remaining = append(remaining, placement)
			}
		}
		if _, _, _, _, _, ok := transportDirectory(t, remaining...).CallStageTransport(); ok {
			t.Fatalf("a directory declaring no write at stage %d still produced a transport plan", withdrawn)
		}
	}
}

func TestTransportKeyIsTheLowestDeclaredWriterOfItsAxis(t *testing.T) {
	directory := transportDirectory(t, declaredCallStagePlacements()...)
	for axis, want := range map[schema.Key]schema.Key{"value": "raw-get", "pack": "pack-source", "heap": "heap-ingress", "call": "call-dispatch", "effect": "effect-body"} {
		key, ok := directory.TransportKey(axis)
		if !ok || key != want {
			t.Fatalf("transport key for axis %q = %q/%v, want %q", axis, key, ok, want)
		}
	}
	if key, ok := directory.TransportKey("probe"); ok || key.Available() {
		t.Fatalf("an axis no mounted rule writes named transport key %q", key)
	}
	if key, _ := directory.TransportKey("call"); key == "call-activation" {
		t.Fatal("a non-transport declaration named the call axis")
	}
}
