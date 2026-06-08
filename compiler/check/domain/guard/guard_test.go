package guard_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractTypeEqualityProbe(t *testing.T) {
	target := &ast.AttrGetExpr{
		Object: &ast.IdentExpr{Value: "page"},
		Key:    &ast.StringExpr{Value: "placement"},
	}
	expr := &ast.RelationalOpExpr{
		Operator: "==",
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{target},
		},
		Rhs: &ast.StringExpr{Value: "string"},
	}

	probe, ok := guard.ExtractTypeEqualityProbe(expr)
	if !ok {
		t.Fatal("expected type equality probe")
	}
	if probe.Expr != target {
		t.Fatal("expected probe expression to be preserved")
	}
	if probe.Key != narrow.BuiltinTypeKey("string") {
		t.Fatalf("probe key = %v, want string key", probe.Key)
	}
	if got := guard.TypeForTypeKey(probe.Key); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("probe type = %v, want string", got)
	}
}

func TestEvaluateTypeProbeComparison_ProvesDisjointFalse(t *testing.T) {
	cmp := typeProbeComparison("==", &ast.StringExpr{Value: "merge"}, "table")
	got := guard.EvaluateTypeProbeComparison(typ.LiteralString("merge"), cmp)
	if got != typ.False {
		t.Fatalf("type(\"merge\") == \"table\" = %v, want false", got)
	}
}

func TestEvaluateTypeProbeComparison_ProvesSubtypeTrue(t *testing.T) {
	cmp := typeProbeComparison("==", &ast.StringExpr{Value: "merge"}, "string")
	got := guard.EvaluateTypeProbeComparison(typ.LiteralString("merge"), cmp)
	if got != typ.True {
		t.Fatalf("type(\"merge\") == \"string\" = %v, want true", got)
	}
}

func TestEvaluateTypeProbeComparison_KeepsUnionUncertain(t *testing.T) {
	cmp := typeProbeComparison("==", &ast.IdentExpr{Value: "content"}, "table")
	observed := typ.NewUnion(typ.String, typ.NewRecord().Field("text", typ.String).Build())
	got := guard.EvaluateTypeProbeComparison(observed, cmp)
	if got != typ.Boolean {
		t.Fatalf("type(string|table) == \"table\" = %v, want boolean", got)
	}
}

func TestEvaluateTypeProbeComparison_ProvesNotEqualTrue(t *testing.T) {
	cmp := typeProbeComparison("~=", &ast.StringExpr{Value: "merge"}, "table")
	got := guard.EvaluateTypeProbeComparison(typ.LiteralString("merge"), cmp)
	if got != typ.True {
		t.Fatalf("type(\"merge\") ~= \"table\" = %v, want true", got)
	}
}

func typeProbeComparison(op string, target ast.Expr, typeName string) guard.TypeProbeComparison {
	expr := &ast.RelationalOpExpr{
		Operator: op,
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{target},
		},
		Rhs: &ast.StringExpr{Value: typeName},
	}
	cmp, ok := guard.ExtractTypeProbeComparison(expr)
	if !ok {
		panic("test constructed invalid type probe")
	}
	return cmp
}
