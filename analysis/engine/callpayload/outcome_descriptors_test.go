package callpayload

import (
	"reflect"
	"testing"
)

// TestCallOutcomeDescriptorsDeriveHandWiredLanes proves the descriptor table
// derives lanes structurally identical to the hand-wired callOutcomeLanes
// registry, in the same order, including the post-return flag and per-field
// presence predicate behavior.
func TestCallOutcomeDescriptorsDeriveHandWiredLanes(t *testing.T) {
	derived := derivedCallOutcomeLanes()
	if len(derived) != len(callOutcomeLanes) {
		t.Fatalf("derived lanes = %d, want hand-wired = %d", len(derived), len(callOutcomeLanes))
	}
	for i, hand := range callOutcomeLanes {
		if derived[i].fieldName != hand.fieldName {
			t.Fatalf("lane[%d] field = %q, want %q", i, derived[i].fieldName, hand.fieldName)
		}
		if derived[i].postReturn != hand.postReturn {
			t.Fatalf("lane[%d] %s postReturn = %v, want %v", i, hand.fieldName, derived[i].postReturn, hand.postReturn)
		}
		outcome := callOutcomeWithOneField(t, hand.fieldName)
		if derived[i].has(outcome) != hand.has(outcome) {
			t.Fatalf("lane[%d] %s has() mismatch for populated field", i, hand.fieldName)
		}
		empty := CallOutcome{}
		if derived[i].has(empty) != hand.has(empty) {
			t.Fatalf("lane[%d] %s has() mismatch for empty outcome", i, hand.fieldName)
		}
	}
}

// TestCallOutcomeDescriptorsEmptyAndEvidenceParity proves descriptor-derived
// Empty and HasPostReturnEvidence match the public methods for the empty
// outcome and for every single-field-populated outcome.
func TestCallOutcomeDescriptorsEmptyAndEvidenceParity(t *testing.T) {
	typ := reflect.TypeOf(CallOutcome{})

	check := func(name string, o CallOutcome) {
		t.Helper()
		if got, want := derivedCallOutcomeEmpty(o), o.Empty(); got != want {
			t.Fatalf("%s: derived Empty = %v, want %v", name, got, want)
		}
		if got, want := derivedCallOutcomeHasPostReturnEvidence(o), o.HasPostReturnEvidence(); got != want {
			t.Fatalf("%s: derived HasPostReturnEvidence = %v, want %v", name, got, want)
		}
	}

	check("empty", CallOutcome{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		check(field.Name, callOutcomeWithOneField(t, field.Name))
	}
}

// TestCallOutcomeDescriptorsWireRefs pins the manifest wire lane cross-reference:
// only ReturnPresenceRelations lowers 1:1 from a wire lane; NormalReturnFacts is
// a nested family and every other field is caller-relative evidence with no
// OperationalEffects wire lane.
func TestCallOutcomeDescriptorsWireRefs(t *testing.T) {
	want := map[string][]string{
		"ReturnPresenceRelations": {"ReturnPresenceRelations"},
	}
	for _, d := range CallOutcomeDescriptors() {
		expected := want[string(d.Kind)]
		if !reflect.DeepEqual(d.WireRef, expected) {
			t.Fatalf("kind %q wire ref = %#v, want %#v", d.Kind, d.WireRef, expected)
		}
	}
}
