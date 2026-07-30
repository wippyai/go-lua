package front

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func TestDynamicStoreTermFailsClosedForAbsentOperand(t *testing.T) {
	body := wir.NewBody("dynamic-store")
	nilRef := body.InternConst(wir.Const{Kind: wir.ConstNil})

	missing, err := dynamicStoreTerm(body, wir.Operand{})
	if err != nil {
		t.Fatalf("absent dynamic store operand: %v", err)
	}
	if got := string(missing.Encoding); got != "scalar/top" {
		t.Fatalf("absent dynamic store operand = %q, want scalar/top", got)
	}

	nilValue, err := dynamicStoreTerm(body, wir.Operand{Kind: wir.OperandConst, Ref: uint32(nilRef)})
	if err != nil {
		t.Fatalf("nil dynamic store operand: %v", err)
	}
	if got := string(nilValue.Encoding); got != "scalar/nil" {
		t.Fatalf("nil dynamic store operand = %q, want scalar/nil", got)
	}
}
