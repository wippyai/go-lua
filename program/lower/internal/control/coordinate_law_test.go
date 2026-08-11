package control

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
)

func TestSpanRejectsMalformedASTCoordinate(t *testing.T) {
	node := &ast.Node{}
	node.SetLine(1)
	node.SetColumn(1)
	node.SetLastLine(1)
	node.SetLastColumn(0)
	writer := &Writer{sourceName: "control.lua"}
	if got := writer.span(node); got != sourcecoord.Invalid("control.lua") {
		t.Fatalf("span = %#v, want invalid sentinel", got)
	}
}
