package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticRuntimeAndTypeShapeRejectTypedNilNodes(t *testing.T) {
	var primitive *ast.PrimitiveTypeExpr
	if validTypeExpr(primitive) {
		t.Fatal("validTypeExpr accepted a typed-nil primitive")
	}
	var identifier *ast.IdentExpr
	if validRuntimeExpr(identifier) {
		t.Fatal("validRuntimeExpr accepted a typed-nil identifier")
	}
	if err := (&Writer{}).ready(); err == nil {
		t.Fatal("ready accepted an uninitialized static writer")
	}
}

func TestStaticPublishRecordsClosedResult(t *testing.T) {
	phases := &continuation.Stack{}
	w := Writer{phases: phases}
	if err := w.publish(23, nil); err != nil {
		t.Fatal(err)
	}
	if got, open := phases.Result(); got != 23 || open {
		t.Fatalf("published static result = %d/%t, want 23/false", got, open)
	}
}
