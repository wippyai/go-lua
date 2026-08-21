package calltarget

import "testing"

func TestBuildRequiresCanonicalBodyBundle(t *testing.T) {
	rows, fault := Build(Input{})
	if rows != nil || !fault.Failed() || fault.Reason() != ReasonUnavailable || fault.Row() != -1 || fault.Subrow() != -1 {
		t.Fatalf("unexpected empty-input result: rows=%v fault=%+v", rows, fault)
	}
}
