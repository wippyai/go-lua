package effectlowering

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCallableIteratorSignatureClassifiesFunctionResultNotContainer(t *testing.T) {
	iterator := typ.Func().Returns(typ.String).Build()
	sig := signature.Function{Type: typ.Func().Param("source", typ.String).Returns(iterator).Build()}
	got, ok := CallableIteratorSignature(sig)
	if !ok || got != iterator {
		t.Fatalf("callable iterator = %p/%t, want %p", got, ok, iterator)
	}
	if _, ok := CallableIteratorSignature(signature.Function{Type: typ.Func().Returns(typ.String).Build()}); ok {
		t.Fatal("ordinary scalar result classified as callable iterator")
	}
}
