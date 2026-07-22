package service

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
)

func TestObservationContractDeclaresOnlyIDEQueryReadClosure(t *testing.T) {
	contract := observationContract()
	if contract.FullResultV1() || contract.SummaryV1() {
		t.Fatalf("IDE query contract retained a coarse closure: %#v", contract)
	}
	want := []transformer.ObservationClass{
		transformer.ObservationClassCallOutcome,
		transformer.ObservationClassEdgeReachability,
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
