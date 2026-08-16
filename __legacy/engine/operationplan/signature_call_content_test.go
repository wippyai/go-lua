package operationplan

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestSignatureCallOperationExposesImmutableContentIdentity(t *testing.T) {
	sig := signature.Function{Type: typ.Func().Returns(typ.String).Build()}
	op, ok := NewSignatureCallOperation(sig)
	if !ok || !op.ContentID().Available() {
		t.Fatal("signature call operation has no semantic content identity")
	}
	want := op.ContentID()
	sig.Type.Returns[0] = typ.Number
	if got := op.ContentID(); got != want {
		t.Fatalf("caller mutation changed sealed identity: %x -> %x", want, got)
	}
	if got := op.clone().ContentID(); got != want {
		t.Fatalf("clone identity = %x, want %x", got, want)
	}
}

func TestSignatureCallOperationSeparatesAllocationTemplateIdentity(t *testing.T) {
	sig := signature.Function{
		Type: typ.Func().Returns(typ.String).Build(),
		OperationalEffects: &signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        "root",
			Objects:     []signature.AllocationObjectTemplate{{ID: "root", Type: typ.String}},
		}}},
	}
	op, ok := NewSignatureCallOperation(sig)
	if !ok || !op.AllocationContentID().Available() {
		t.Fatal("allocation-bearing call has no separately composable allocation identity")
	}
	without, ok := NewSignatureCallOperation(signature.Function{Type: sig.Type})
	if !ok || without.AllocationContentID().Available() {
		t.Fatal("allocation-free call unexpectedly has allocation identity")
	}
}

func TestSignatureCallOperationFailsClosedWhenIdentityUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if op, ok := NewSignatureCallOperationContext(ctx, signature.Function{Type: typ.Func().Build()}); ok || op.valid() {
		t.Fatal("canceled identity admitted a signature operation")
	}
}
