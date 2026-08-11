package table

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
)

func TestSpanRejectsMalformedASTCoordinate(t *testing.T) {
	node := &ast.Node{}
	node.SetLine(2)
	node.SetColumn(1)
	node.SetLastLine(1)
	node.SetLastColumn(2)
	writer := &Writer{sourceName: "table.lua"}
	if got := writer.span(node); got != sourcecoord.Invalid("table.lua") {
		t.Fatalf("span = %#v, want invalid sentinel", got)
	}
}
