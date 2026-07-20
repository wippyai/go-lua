package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// TestGenericForLoopVarNegatedDiscriminantEdgeNarrowsRoot proves the else edge of
// a discriminant equality guard narrows an un-annotated generic-for loop variable
// to the complementary variant. The else-branch read item.payment_id must project
// the refund arm's required field as Present, not the union's optional string?.
// TestGenericForStatelessFunctionIteratorNarrowsFirstVariable proves the loop
// variable of a stateless function iterator (for w in f do, where f returns the
// iterator fun(): string?) is typed from the iterator function's result, narrowed
// to its non-nil form for the first variable. gmatch returns fun(): string?, so w
// is string inside the body. This is the type that makes `local ok: string = w`
// check clean and `local n: number = w` report a type error.
func assertExpressionTypeAtBoundary(t *testing.T, reg *axis.Registry, result *Result, local *ast.LocalAssignStmt, want typ.Type) {
	t.Helper()
	point := requireLocalAssignmentPoint(t, result, local, 0)
	got, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0])
	if !ok {
		t.Fatalf("ExpressionValueAtBoundary returned false for %#v", local.Exprs[0])
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("expression type = %v/%v, want %v", gotType, ok, want)
	}
}
