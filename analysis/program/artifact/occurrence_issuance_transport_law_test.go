package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// transportDeclaration states one synthetic rule declaration for the transport
// laws: the key it is declared under, the factor axis it writes, and the stage
// its subscription is issued at.
type transportDeclaration struct {
	key       schema.Key
	writes    schema.Key
	stage     RuleStage
	transport bool
}

func transportDirectory(declarations ...transportDeclaration) IssuanceDirectory {
	directory := make(IssuanceDirectory, 0, len(declarations))
	for _, declaration := range declarations {
		placement := IssuancePlacement{
			Occurrence:  OccurrenceCall,
			Form:        IssuanceFormBase,
			Input:       RuleInputNone,
			Stage:       declaration.stage,
			Requirement: IssuanceRequirementUnrestricted,
			Key:         declaration.key,
			Writes:      declaration.writes,
			Transport:   declaration.transport,
		}
		if declaration.stage != RuleStageBase {
			placement.Form, placement.Input = IssuanceFormCallStage, RuleInputFinish
		}
		directory = append(directory, placement)
	}
	return directory
}

// declaredCallStageTransport mirrors the sealed composition's shape: three
// base-stage axes, one axis produced at the call-dispatch stage, and one
// produced at the call-effect stage by more than one rule.
func declaredCallStageTransport() IssuanceDirectory {
	return transportDirectory(
		transportDeclaration{key: "value-source", writes: "value", stage: RuleStageBase, transport: true},
		transportDeclaration{key: "raw-get", writes: "value", stage: RuleStageLocal, transport: true},
		transportDeclaration{key: "pack-source", writes: "pack", stage: RuleStageBase, transport: true},
		transportDeclaration{key: "heap-ingress", writes: "heap", stage: RuleStageBase, transport: true},
		transportDeclaration{key: "call-dispatch", writes: "call", stage: RuleStageCallDispatch, transport: true},
		transportDeclaration{key: "call-activation", writes: "call", stage: RuleStageCallSummary, transport: false},
		transportDeclaration{key: "effect-selected", writes: "effect", stage: RuleStageCallEffect, transport: true},
		transportDeclaration{key: "effect-body", writes: "effect", stage: RuleStageCallEffect, transport: true},
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
	plan, planOK := declaredCallStageTransport().callStageTransport()
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
	directory := append(declaredCallStageTransport(), transportDirectory(
		transportDeclaration{key: "probe-source", writes: "probe", stage: RuleStageBase, transport: true},
	)...)
	plan, planOK := directory.callStageTransport()
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
	for _, withdrawn := range []RuleStage{RuleStageCallDispatch, RuleStageCallEffect} {
		remaining := make(IssuanceDirectory, 0, len(declaredCallStageTransport()))
		for _, placement := range declaredCallStageTransport() {
			if placement.Stage != withdrawn {
				remaining = append(remaining, placement)
			}
		}
		if _, planOK := remaining.callStageTransport(); planOK {
			t.Fatalf("a directory declaring no write at stage %d still produced a transport plan", withdrawn)
		}
	}
}

// TestTransportKeyIsTheLowestDeclaredWriterOfItsAxis pins the naming rule the
// transports share: one axis is named by one declared key, whichever arm of the
// compiler transports it.
func TestTransportKeyIsTheLowestDeclaredWriterOfItsAxis(t *testing.T) {
	directory := declaredCallStageTransport()
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
