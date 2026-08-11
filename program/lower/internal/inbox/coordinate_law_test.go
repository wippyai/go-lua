package inbox

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
)

func TestExpressionAndTypeSpansRejectMalformedCoordinates(t *testing.T) {
	expression := &ast.NumberExpr{}
	expression.SetLine(-1)
	expression.SetColumn(1)
	expression.SetLastLine(1)
	expression.SetLastColumn(2)
	if _, ok := ExpressionSpan(expression, "inbox.lua"); ok {
		t.Fatal("ExpressionSpan accepted a negative source coordinate")
	}

	typeExpr := &ast.PrimitiveTypeExpr{}
	typeExpr.SetLine(1)
	typeExpr.SetColumn(1)
	typeExpr.SetLastLine(1)
	typeExpr.SetLastColumn(0)
	if _, ok := TypeSpan(typeExpr, "inbox.lua"); ok {
		t.Fatal("TypeSpan accepted a mixed-zero source extent")
	}
}

func TestExpressionPushRejectsInvalidSentinelWithoutContinuation(t *testing.T) {
	expression := &ast.NumberExpr{}
	expression.SetLine(-1)
	expression.SetColumn(1)
	expression.SetLastLine(1)
	expression.SetLastColumn(2)
	phases := new(phase.Stack)
	queue := NewExpressions(phases)
	if err := queue.Push(expression, 1, sourcecoord.Invalid("inbox.lua")); err == nil {
		t.Fatal("Push accepted malformed expression sentinel")
	}
	if !phases.Clean() || !queue.Clean() {
		t.Fatal("rejected expression left continuation state")
	}
}
