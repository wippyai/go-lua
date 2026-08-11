package function

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
)

func TestSpanRejectsMalformedASTCoordinate(t *testing.T) {
	node := &ast.Node{}
	node.SetLine(1)
	node.SetColumn(1)
	node.SetLastLine(0)
	node.SetLastColumn(2)
	writer := &Writer{sourceName: "function.lua"}
	if got := writer.span(node); got != sourcecoord.Invalid("function.lua") {
		t.Fatalf("span = %#v, want invalid sentinel", got)
	}
}
