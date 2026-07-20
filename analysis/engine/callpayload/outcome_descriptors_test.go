package callpayload

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

// TestCallOutcomeDescriptorsDeriveCanonicalLanes verifies descriptor and
// canonical lane order, post-return flags, and presence predicates.
func TestCallOutcomeDescriptorsDeriveCanonicalLanes(t *testing.T) {
	derived := derivedCallOutcomeLanes()
	if len(derived) != len(callOutcomeLanes) {
		t.Fatalf("derived lanes = %d, want canonical = %d", len(derived), len(callOutcomeLanes))
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

func derivedCallOutcomeEmpty(o CallOutcome) bool {
	for _, lane := range derivedCallOutcomeLanes() {
		if lane.has(o) {
			return false
		}
	}
	return true
}

func derivedCallOutcomeHasPostReturnEvidence(o CallOutcome) bool {
	for _, lane := range derivedCallOutcomeLanes() {
		if lane.postReturn && lane.has(o) {
			return true
		}
	}
	return false
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
// SuspensionKnown, MaySuspend, and ReturnPresenceRelations lower 1:1 from wire lanes;
// NormalReturnFacts is a nested family and every other field is caller-relative
// evidence or local certification metadata with no OperationalEffects wire lane.
func TestCallOutcomeDescriptorsWireRefs(t *testing.T) {
	want := map[string][]string{
		"SuspensionKnown":         {"SuspensionKnown"},
		"MaySuspend":              {"MaySuspend"},
		"ReturnPresenceRelations": {"ReturnPresenceRelations"},
		"TypestateRequirements":   {"TypestateRequirements"},
	}
	for _, d := range CallOutcomeDescriptors() {
		expected := want[string(d.Kind)]
		if !reflect.DeepEqual(d.WireRef, expected) {
			t.Fatalf("kind %q wire ref = %#v, want %#v", d.Kind, d.WireRef, expected)
		}
	}
}

// TestCallOutcomeDescriptorsOwnExternalTransactionMetadata proves the storage
// descriptor is also the sole exhaustive owner of external-call transaction
// access. Site selection may narrow that table, but no consumer reclassifies
// field names or reconstructs a lane matrix.
func TestCallOutcomeDescriptorsOwnExternalTransactionMetadata(t *testing.T) {
	descriptors := CallOutcomeDescriptors()
	roles := CallOutcomeFieldRoles()
	if len(descriptors) != len(roles) {
		t.Fatalf("descriptors = %d, roles = %d", len(descriptors), len(roles))
	}
	for index, descriptor := range descriptors {
		if descriptor.Ops.transaction == nil || !descriptor.Ops.transaction.classified {
			t.Fatalf("descriptor %q has no transaction classification", descriptor.Kind)
		}
		if roles[index].FieldName != string(descriptor.Kind) || roles[index].transaction == nil || !roles[index].transaction.classified {
			t.Fatalf("role[%d] = %#v, descriptor = %q", index, roles[index], descriptor.Kind)
		}
		if !reflect.DeepEqual(roles[index].transaction.lanes.IDs(), descriptor.Ops.transaction.lanes.IDs()) {
			t.Fatalf("role %q lanes = %v, descriptor lanes = %v", descriptor.Kind, roles[index].transaction.lanes.IDs(), descriptor.Ops.transaction.lanes.IDs())
		}
	}

	program := SealCallOutcomeProgram(
		"descriptor transaction test",
		[]string{"ParamConditions", "ProtectedCallTypestate"},
		state.NewLaneSet(), state.NewLaneSet(), nil, nil,
		func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
			return CallOutcome{}, nil
		},
	)
	prepared, err := program.PrepareSite(transfer.NodeContext{}, factflow.CallSiteView{})
	if err != nil {
		t.Fatal(err)
	}
	capability := prepared.Capability()
	if !capability.OperandValueWrites() {
		t.Fatal("selected ParamConditions did not retain descriptor-owned operand writes")
	}
	want := state.NewLaneSet(
		state.LaneTypestates,
		state.LanePathEvidence, state.LaneStoreRelations, state.LaneDiffRelations,
	)
	if got := capability.TransactionLanes().IDs(); !reflect.DeepEqual(got, want.IDs()) {
		t.Fatalf("selected transaction lanes = %v, want %v", got, want.IDs())
	}
}
