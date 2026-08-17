package continuation

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
	"testing"
)

func TestExpressionSpanRejectsTypedNilAndPreservesExactSpan(t *testing.T) {
	span := programsource.Span{File: "expr.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	expr := &ast.NumberExpr{Value: "1"}
	expr.SetLine(1)
	expr.SetColumn(1)
	expr.SetLastLine(1)
	expr.SetLastColumn(2)
	got, ok := ExpressionSpan(expr, span.File)
	if !ok || got != span {
		t.Fatalf("ExpressionSpan = %#v/%v, want %#v/true", got, ok, span)
	}
	var typedNil *ast.NumberExpr
	if _, ok := ExpressionSpan(typedNil, span.File); ok {
		t.Fatal("ExpressionSpan accepted a typed nil")
	}
	queue := NewExpressions(&Stack{})
	if err := queue.Push(expr, keyspace.MakeTerm(keyspace.FamilyBody, 1), span); err != nil {
		t.Fatal(err)
	}
}
