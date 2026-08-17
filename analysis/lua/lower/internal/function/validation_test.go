package function

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestFunctionTargetAcceptsDottedNamesOnly(t *testing.T) {
	root := &ast.IdentExpr{Value: "object"}
	key := &ast.StringExpr{Value: "field"}
	dotted := &ast.AttrGetExpr{Object: root, Key: key, KeySyntax: ast.AttrKeyDot}
	if !functionTarget(dotted) {
		t.Fatal("functionTarget rejected a dotted declaration target")
	}
	bracketed := &ast.AttrGetExpr{Object: root, Key: key, KeySyntax: ast.AttrKeyIndex}
	if functionTarget(bracketed) {
		t.Fatal("functionTarget accepted an indexed declaration target")
	}
}

func TestValidMethodDefinitionRequiresAuthoredMethodShape(t *testing.T) {
	fn := &ast.FunctionExpr{}
	name := &ast.FuncName{
		Receiver:       &ast.IdentExpr{Value: "object"},
		Method:         "run",
		MethodPosition: ast.Position{Line: 1, Column: 8, EndLine: 1, EndColumn: 10},
	}
	stmt := &ast.FuncDefStmt{Name: name, Func: fn}
	origin := bind.FunctionOrigin{Func: fn, Kind: bind.FunctionOriginMethod, Method: "run"}
	if err := (&Writer{}).validMethodDef(stmt, origin); err != nil {
		t.Fatalf("validMethodDef rejected a valid method shape: %v", err)
	}
	name.MethodPosition = ast.Position{}
	if err := (&Writer{}).validMethodDef(stmt, origin); err == nil {
		t.Fatal("validMethodDef accepted a method without its authored selector span")
	}
}
