package engine

import "testing"

// A declared activation trigger states the family and application it
// activates under; its candidate set is a separate plane that a program may
// leave empty. A mounted call site placed in a program that declares no
// activatable body is exactly that case, and it is a placement the artifact
// makes statically, before any body inventory exists.
//
// These laws hold the trigger addressable on its own declaration: it is
// admitted, published in the semantic directory, resolvable as a committed
// activation member, and - when the artifact placed it on a native issuance
// cut - addressable as that mounted call stage. Nothing here is conditional
// on a candidate existing.

// TestCandidateFreeActivationTriggerIsAddressable proves the whole trigger
// address survives an empty candidate set.
func TestCandidateFreeActivationTriggerIsAddressable(t *testing.T) {
	fixture := newSelectedOverlayLawFixtureWithOptions(t, selectedOverlayLawOptions{candidateCount: 0})
	if fixture.graph == nil || fixture.solver == nil {
		t.Fatal("a candidate-free activation trigger refused its program")
	}
	locator, located := fixture.graph.directory.activation(fixture.activationID)
	if !located {
		t.Fatal("candidate-free trigger has no activation directory locator")
	}
	if _, resolved := locator.Resolve(fixture.graph.graph); !resolved {
		t.Fatal("candidate-free trigger locator did not resolve its committed row")
	}
	member, ok := fixture.graph.MountedActivationMember(fixture.activationRole, fixture.activationMount, fixture.activationPoint, fixture.activationOccurrence)
	if !ok || member.program != fixture.graph {
		t.Fatal("candidate-free trigger is not addressable as a committed activation member")
	}
}

// TestCandidateFreeActivationTriggerOwnsItsNativeStage proves a native
// issuance cut placed on the trigger role addresses the attached member. The
// native-stage inverse is derived from the artifact placement, so a trigger
// the declaration skipped would leave that placement unaddressable.
func TestCandidateFreeActivationTriggerOwnsItsNativeStage(t *testing.T) {
	for name, candidates := range map[string]int{"candidate-free": 0, "one-candidate": 1} {
		fixture := newSelectedOverlayLawFixtureWithOptions(t, selectedOverlayLawOptions{candidateCount: candidates, nativeStage: true})
		if fixture.graph == nil {
			t.Fatalf("%s: native-staged activation trigger refused its program", name)
		}
		stage, ok := fixture.graph.MountedNativeCallStage(fixture.activationRole, fixture.activationMount, fixture.activationOccurrence)
		if !ok || !stage.Available() {
			t.Fatalf("%s: native call stage of the trigger is not addressable", name)
		}
	}
}

// TestCandidateFreeActivationTriggerStillProjectsItsApplication proves the
// application the trigger activates under is read from the trigger's own
// declaration. A projection recovered from candidate receipts would answer
// nothing here.
func TestCandidateFreeActivationTriggerStillProjectsItsApplication(t *testing.T) {
	fixture := newSelectedOverlayLawFixtureWithOptions(t, selectedOverlayLawOptions{candidateCount: 0})
	if fixture.graph == nil {
		t.Fatal("a candidate-free activation trigger refused its program")
	}
	member, ok := fixture.graph.MountedActivationMember(fixture.activationRole, fixture.activationMount, fixture.activationPoint, fixture.activationOccurrence)
	if !ok {
		t.Fatal("candidate-free trigger is not addressable as a committed activation member")
	}
	ordinal, found := fixture.graph.state.schema.ruleOrdinalOf(member.member.Rule())
	shape, shapeOK := fixture.graph.state.schema.ruleShapeAt(ordinal)
	if !found || !shapeOK || shape.ActivationCount != 1 {
		t.Fatal("committed trigger has no declared activation shape")
	}
	if !fixture.graph.topology.TriggerBound(member.member.Key(), shape.ActivationFamily) {
		t.Fatal("candidate-free trigger is not a bound activation trigger")
	}
	if _, projected := fixture.graph.topology.ActivationApplication(member.member.Key(), shape.ActivationFamily); !projected {
		t.Fatal("candidate-free trigger projected no application")
	}
}
