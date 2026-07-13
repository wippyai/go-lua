package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSignatureIntrinsicIsImmutableOperationContent(t *testing.T) {
	sig := signature.Function{Type: typ.Func().Param("value", typ.Any).Returns(typ.String).Build()}
	plain, ok := NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("plain operation rejected")
	}
	intrinsic, ok := NewSignatureIntrinsicCallOperation(sig, signature.IntrinsicLuaType)
	if !ok {
		t.Fatal("intrinsic operation rejected")
	}
	if plain.ContentID() != intrinsic.ContentID() || plain.AllocationContentID() != intrinsic.AllocationContentID() {
		t.Fatal("adding intrinsic changed canonical signature content IDs")
	}

	graph := cfg.New()
	plainPoint := graph.AddNode(cfg.NodeCall)
	intrinsicPoint := graph.AddNode(cfg.NodeCall)
	plan := New(graph, factflow.FactsInput{}).WithSignatureCalls(map[cfg.Point]SignatureCallOperation{
		plainPoint: plain, intrinsicPoint: intrinsic,
	})
	if len(plan.signatures) != 2 {
		t.Fatalf("distinct operation contents interned as %d descriptors", len(plan.signatures))
	}
	got, exists := plan.SignatureCallOperation(intrinsicPoint)
	identity, exact := got.Intrinsic()
	if !exists || !exact || identity != signature.IntrinsicLuaType || got.ContentID() != intrinsic.ContentID() {
		t.Fatalf("plan projection lost intrinsic/content identity: %#v %v/%v", got, exists, exact)
	}
	if _, accepted := NewSignatureIntrinsicCallOperation(sig, signature.IntrinsicNone); accepted {
		t.Fatal("invalid intrinsic was accepted")
	}
}
