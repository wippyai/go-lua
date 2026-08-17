package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticTypeConstructorsRejectIncompleteInputs(t *testing.T) {
	var w Writer
	if _, err := w.PrimitiveOrRef(nil); err == nil {
		t.Fatal("PrimitiveOrRef accepted a nil type")
	}
	if _, err := w.Array(&ast.ArrayTypeExpr{}, 0); err == nil {
		t.Fatal("Array accepted a zero element term")
	}
	if _, err := w.RuntimeTypeTarget(spanForTest(), bind.RuntimeTypeValue{}); err == nil {
		t.Fatal("RuntimeTypeTarget accepted an empty runtime type value")
	}
}
