package exportmanifest

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
)

func TestObservationContractDeclaresOnlyExportReadClosure(t *testing.T) {
	contract := ObservationContract()
	if contract.FullResultV1() || contract.SummaryV1() {
		t.Fatalf("export contract retained a coarse closure: %#v", contract)
	}
	want := []transformer.ObservationClass{
		transformer.ObservationClassNormalReturn,
		transformer.ObservationClassPathValue,
		transformer.ObservationClassPointState,
	}
	if !reflect.DeepEqual(contract.Classes(), want) {
		t.Fatalf("classes = %#v, want %#v", contract.Classes(), want)
	}
}
