package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSourceParamEvidencePolicy(t *testing.T) {
	typed := functionForPolicyTest("x", &ast.PrimitiveTypeExpr{Name: "string"})
	if SourceParamReceivesCallEvidence(typed, cfg.Build(typed), 0) {
		t.Fatalf("hard source annotations must not receive call evidence")
	}

	dynamic := functionForPolicyTest("x", &ast.PrimitiveTypeExpr{Name: "any"})
	if SourceParamReceivesCallEvidence(dynamic, cfg.Build(dynamic), 0) {
		t.Fatalf("explicit any is a hard dynamic contract and must not receive call evidence")
	}

	soft := functionForPolicyTest("x", &ast.ArrayTypeExpr{Element: &ast.PrimitiveTypeExpr{Name: "any"}})
	if !SourceParamReceivesCallEvidence(soft, cfg.Build(soft), 0) {
		t.Fatalf("soft structural annotation should receive call evidence")
	}

	untyped := functionForPolicyTest("x", nil)
	if !SourceParamReceivesCallEvidence(untyped, cfg.Build(untyped), 0) {
		t.Fatalf("untyped parameters should receive body-effective call evidence")
	}
}

func TestStaticArgumentShapePreservesNestedDiscriminants(t *testing.T) {
	got := StaticArgumentShape(&ast.TableExpr{Fields: []*ast.Field{
		{
			Key: &ast.StringExpr{Value: "kind"},
			Value: &ast.StringExpr{
				Value: "text",
			},
		},
		{
			Key: &ast.StringExpr{Value: "payload"},
			Value: &ast.TableExpr{Fields: []*ast.Field{
				{
					Key:   &ast.StringExpr{Value: "id"},
					Value: &ast.StringExpr{Value: "abc"},
				},
			}},
		},
	}})
	want := typ.NewRecord().
		Field("kind", typ.LiteralString("text")).
		Field("payload", typ.NewRecord().Field("id", typ.LiteralString("abc")).Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("StaticArgumentShape() = %v, want %v", got, want)
	}
}

func functionForPolicyTest(name string, typExpr ast.TypeExpr) *ast.FunctionExpr {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{name}},
	}
	if typExpr != nil {
		fn.ParList.Types = []ast.TypeExpr{typExpr}
	}
	return fn
}
