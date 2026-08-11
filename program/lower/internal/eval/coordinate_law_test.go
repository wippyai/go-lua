package eval

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower/internal/inbox"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
)

func TestEnqueueExpressionRejectsMalformedASTCoordinate(t *testing.T) {
	node := &ast.NumberExpr{}
	node.SetLine(-1)
	node.SetColumn(1)
	node.SetLastLine(1)
	node.SetLastColumn(2)
	phases := new(phase.Stack)
	values := Values{expressions: inbox.NewExpressions(phases)}
	if err := values.enqueueExpression(node, keyspace.Term(1), sourcecoord.Invalid("eval.lua")); err == nil {
		t.Fatal("enqueueExpression accepted malformed AST coordinate")
	}
	if !phases.Clean() {
		t.Fatal("rejected expression left continuation state")
	}
}
