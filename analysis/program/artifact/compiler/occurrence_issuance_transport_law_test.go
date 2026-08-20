package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// transportDeclaration states one synthetic rule declaration for the transport
// laws: the key it is declared under, the factor axis it writes, and the stage
// its subscription is issued at.
type transportDeclaration struct {
	key       schema.Key
	writes    schema.Key
	stage     programschema.RuleStage
	transport bool
}

// declaredStageFramings is the framing every staged cut is declared under. The
// fixtures state it once, from the pinned spelling the identity laws hold.
func declaredStageFramings() (map[IssuanceForm]string, map[programschema.RuleStage]string) {
	return map[IssuanceForm]string{
			IssuanceFormLocal:            pinnedLocalStageFraming,
			IssuanceFormComputation:      pinnedLocalComputationStageFraming,
			IssuanceFormLocalPredecessor: pinnedLocalPredecessorStageFraming,
		}, map[programschema.RuleStage]string{
			programschema.RuleStageCallDispatch: pinnedCallDispatchStageFraming,
			programschema.RuleStageCallSummary:  pinnedCallSummaryStageFraming,
			programschema.RuleStageCallEffect:   pinnedCallEffectStageFraming,
		}
}

func transportPlacements(declarations ...transportDeclaration) []IssuancePlacement {
	placements := make([]IssuancePlacement, 0, len(declarations))
	for _, declaration := range declarations {
		placement := IssuancePlacement{
			Occurrence:  programschema.OccurrenceCall,
			Form:        IssuanceFormBase,
			Input:       programschema.RuleInputNone,
			Stage:       declaration.stage,
			Requirement: IssuanceRequirementUnrestricted,
			Key:         declaration.key,
			Writes:      declaration.writes,
			Transport:   declaration.transport,
		}
		if declaration.stage != programschema.RuleStageBase {
			placement.Form, placement.Input = IssuanceFormCallStage, programschema.RuleInputFinish
		}
		placements = append(placements, placement)
	}
	return placements
}

func transportDirectory(t *testing.T, placements ...IssuancePlacement) IssuanceDirectory {
	t.Helper()
	forms, stages := declaredStageFramings()
	directory, ok := NewIssuanceDirectory(placements, forms, stages)
	if !ok {
		t.Fatal("the declared placements and framings were refused admission")
	}
	return directory
}

// declaredCallStageTransport mirrors the sealed composition's shape: three
// base-stage axes, one axis produced at the call-dispatch stage, and one
// produced at the call-effect stage by more than one rule.
func declaredCallStagePlacements() []IssuancePlacement {
	return transportPlacements(
		transportDeclaration{key: "value-source", writes: "value", stage: programschema.RuleStageBase, transport: true},
		transportDeclaration{key: "raw-get", writes: "value", stage: programschema.RuleStageLocal, transport: true},
		transportDeclaration{key: "pack-source", writes: "pack", stage: programschema.RuleStageBase, transport: true},
		transportDeclaration{key: "heap-ingress", writes: "heap", stage: programschema.RuleStageBase, transport: true},
		transportDeclaration{key: "call-dispatch", writes: "call", stage: programschema.RuleStageCallDispatch, transport: true},
		transportDeclaration{key: "call-activation", writes: "call", stage: programschema.RuleStageCallSummary, transport: false},
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

// TestCallStageTransportIsDerivedFromDeclaredWrites is the G4 law at the
// directory: the call-stage transport plan names the declared transport key of
// every mounted axis, split by which stage produces the axis. No key list is
// authored, so a declaration change moves the plan with it.
func TestCallStageTransportIsDerivedFromDeclaredWrites(t *testing.T) {
	plan, planOK := transportDirectory(t, declaredCallStagePlacements()...).callStageTransport()
	if !planOK {
		t.Fatal("the declared directory produced no call-stage transport plan")
	}
	assertKeys(t, "dispatch entry", plan.dispatchEntry, "call-dispatch", "heap-ingress", "pack-source", "raw-get")
	assertKeys(t, "effect bypass", plan.effectBypass, "effect-body")
	assertKeys(t, "dispatch forward", plan.dispatchForward, "call-dispatch")
}

// TestCallStageTransportFollowsAnAddedDeclaredAxis proves the plan carries a
// mounted axis the compiler was never told about by name.
func TestCallStageTransportFollowsAnAddedDeclaredAxis(t *testing.T) {
	extended := append(declaredCallStagePlacements(), transportPlacements(
		transportDeclaration{key: "probe-source", writes: "probe", stage: programschema.RuleStageBase, transport: true},
	)...)
	plan, planOK := transportDirectory(t, extended...).callStageTransport()
	if !planOK {
		t.Fatal("the extended directory produced no call-stage transport plan")
	}
	assertKeys(t, "dispatch entry", plan.dispatchEntry, "call-dispatch", "heap-ingress", "pack-source", "probe-source", "raw-get")
	assertKeys(t, "effect bypass", plan.effectBypass, "effect-body")
	assertKeys(t, "dispatch forward", plan.dispatchForward, "call-dispatch")
}

// TestCallStageTransportRefusesAStageWithNoDeclaredWrite is the red proof: the
// plan is the declarations, so withdrawing the write a call stage declares
// leaves no plan to compile from rather than a stale authored list.
func TestCallStageTransportRefusesAStageWithNoDeclaredWrite(t *testing.T) {
	for _, withdrawn := range []programschema.RuleStage{programschema.RuleStageCallDispatch, programschema.RuleStageCallEffect} {
		declared := declaredCallStagePlacements()
		remaining := make([]IssuancePlacement, 0, len(declared))
		for _, placement := range declared {
			if placement.Stage != withdrawn {
				remaining = append(remaining, placement)
			}
		}
		if _, planOK := transportDirectory(t, remaining...).callStageTransport(); planOK {
			t.Fatalf("a directory declaring no write at stage %d still produced a transport plan", withdrawn)
		}
	}
}

// TestTransportKeyIsTheLowestDeclaredWriterOfItsAxis pins the naming rule the
// transports share: one axis is named by one declared key, whichever arm of the
// compiler transports it.
func TestTransportKeyIsTheLowestDeclaredWriterOfItsAxis(t *testing.T) {
	directory := transportDirectory(t, declaredCallStagePlacements()...)
	for axis, want := range map[schema.Key]schema.Key{"value": "raw-get", "pack": "pack-source", "heap": "heap-ingress", "call": "call-dispatch", "effect": "effect-body"} {
		key, keyOK := directory.TransportKey(axis)
		if !keyOK || key != want {
			t.Fatalf("transport key for axis %q = %q/%v, want %q", axis, key, keyOK, want)
		}
	}
	if key, keyOK := directory.TransportKey("probe"); keyOK || key.Available() {
		t.Fatalf("an axis no mounted rule writes named transport key %q", key)
	}
	// The activation lane declares no transport, so it never names an axis.
	if key, _ := directory.TransportKey("call"); key == "call-activation" {
		t.Fatal("a non-transport declaration named the call axis")
	}
}
