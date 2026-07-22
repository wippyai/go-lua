package diagnostics

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
)

func TestDiscriminatedUnionObservationContractDeclaresOnlyItsReadClosure(t *testing.T) {
	contract := DiscriminatedUnionObservationContract()
	if contract.FullResultV1() || contract.SummaryV1() {
		t.Fatalf("discriminated-union contract retained a coarse closure: %#v", contract)
	}
	want := []transformer.ObservationClass{
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	}
	if !reflect.DeepEqual(contract.Classes(), want) {
		t.Fatalf("classes = %#v, want %#v", contract.Classes(), want)
	}
}

func TestLifecycleResourceObservationContractDeclaresOnlyItsReadClosure(t *testing.T) {
	contract := LifecycleResourceObservationContract()
	if contract.FullResultV1() || contract.SummaryV1() {
		t.Fatalf("lifecycle contract retained a coarse closure: %#v", contract)
	}
	want := []transformer.ObservationClass{
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEntryExitState,
		transformer.ObservationClassNodeOutput,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	}
	if !reflect.DeepEqual(contract.Classes(), want) {
		t.Fatalf("classes = %#v, want %#v", contract.Classes(), want)
	}
}

func TestNilSafetyPresenceObservationContractDeclaresOnlyItsReadClosure(t *testing.T) {
	contract := NilSafetyPresenceObservationContract()
	if contract.FullResultV1() || contract.SummaryV1() {
		t.Fatalf("nil-safety contract retained a coarse closure: %#v", contract)
	}
	want := []transformer.ObservationClass{
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEdgeReachability,
		transformer.ObservationClassNodeOutput,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	}
	if !reflect.DeepEqual(contract.Classes(), want) {
		t.Fatalf("classes = %#v, want %#v", contract.Classes(), want)
	}
}

func TestTypeAssignmentObservationContractDeclaresOnlyItsReadClosure(t *testing.T) {
	contract := TypeAssignmentObservationContract()
	if contract.FullResultV1() || contract.SummaryV1() {
		t.Fatalf("type-assignment contract retained a coarse closure: %#v", contract)
	}
	want := []transformer.ObservationClass{
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEntryExitState,
		transformer.ObservationClassNodeOutput,
		transformer.ObservationClassNormalReturn,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	}
	if !reflect.DeepEqual(contract.Classes(), want) {
		t.Fatalf("classes = %#v, want %#v", contract.Classes(), want)
	}
}
