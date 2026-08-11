package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
)

func TestSpanRejectsMalformedASTCoordinate(t *testing.T) {
	node := &ast.Node{}
	node.SetLine(-1)
	node.SetColumn(1)
	node.SetLastLine(1)
	node.SetLastColumn(2)
	writer := &Writer{file: "call.lua"}
	if got := writer.span(node); got != sourcecoord.Invalid("call.lua") {
		t.Fatalf("span = %#v, want invalid sentinel", got)
	}
}
