package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestAssemblyUnaryOperatorLinksAuthoredOperand(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	operand := c.Integer(assemblyTestSpan(), body, 1)
	term := c.Unary(assemblyTestSpan(), body, kind.UnaryNeg, operand)
	if term == 0 {
		t.Fatal("Unary rejected a valid authored operand")
	}
}
