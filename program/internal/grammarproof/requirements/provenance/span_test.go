package provenance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	programlower "github.com/wippyai/go-lua/program/lower"
)

// TestExactSourceCoordinateLaw makes the coordinate rule executable without
// inspecting lowerer composition. The Binary Term is selected by its public
// typed relation; Exact then proves it retained the whole parser occurrence,
// while the child span must not be accepted as the parent occurrence.
func TestExactSourceCoordinateLaw(t *testing.T) {
	const file = "provenance.lua"
	const source = "return left + right"
	statements, err := parse.ParseString(source, file)
	if err != nil {
		t.Fatal(err)
	}
	if bound := bind.BindChunk(statements); bound == nil {
		t.Fatal("public binder returned nil result")
	}
	p, err := programlower.Lower(programlower.Source{Name: file, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	returned := statements[0].(*ast.ReturnStmt)
	expression := returned.Exprs[0].(*ast.ArithmeticOpExpr)
	binaries := p.Flow().Authored().Operators().Binaries()
	term, ok := binaries.At(0)
	if !ok {
		t.Fatal("missing public Binary Term")
	}
	_, op, left, _, ok := binaries.Get(term)
	if !ok || op != flowkind.BinaryAdd || left == 0 {
		t.Fatalf("Binary = op %d left %v ok %v", op, left, ok)
	}
	if err := Exact(p.Source().Identity(), term, expression, file); err != nil {
		t.Fatalf("exact source coordinate: %v", err)
	}
	if err := Exact(p.Source().Identity(), term, expression.Lhs, file); err == nil {
		t.Fatal("parent Binary Term accepted child source coordinate")
	}
	if err := Exact(p.Source().Identity(), left, expression.Lhs, file); err != nil {
		t.Fatalf("exact left source coordinate: %v", err)
	}
}
