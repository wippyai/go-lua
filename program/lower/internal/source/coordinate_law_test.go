package source

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
	node.SetLastColumn(-1)
	owner := &Owner{name: "source.lua"}
	if got := owner.span(node); got != sourcecoord.Invalid("source.lua") {
		t.Fatalf("span = %#v, want invalid sentinel", got)
	}
}
