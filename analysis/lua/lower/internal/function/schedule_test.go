package function

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestScheduleExprRequiresOwnedHostAndSource(t *testing.T) {
	w := Writer{}
	fn := &ast.FunctionExpr{}
	if err := w.ScheduleExpr(fn, 0, source.Span{File: "function.lua"}); err == nil {
		t.Fatal("ScheduleExpr accepted a zero host")
	}
	if err := w.ScheduleExpr(fn, 1, source.Span{}); err == nil {
		t.Fatal("ScheduleExpr accepted an empty source span")
	}
}

func TestScheduleDefRejectsMissingDefinitionNode(t *testing.T) {
	w := Writer{}
	if err := w.ScheduleDef(nil, 1, source.Span{File: "function.lua"}, source.Span{File: "function.lua"}); err == nil {
		t.Fatal("ScheduleDef accepted a missing declaration")
	}
}
