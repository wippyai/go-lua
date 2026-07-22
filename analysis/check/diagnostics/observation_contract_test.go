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
		transformer.ObservationClassPointReachability,
		transformer.ObservationClassPointState,
	}
	if !reflect.DeepEqual(contract.Classes(), want) {
		t.Fatalf("classes = %#v, want %#v", contract.Classes(), want)
	}
}
