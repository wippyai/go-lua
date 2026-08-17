package closed

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// EvaluateClosedForTest is an internal-test-only bridge. It keeps the
// production evaluator unexported while allowing the external receipt fixture
// to prove the exact semantic successor without importing grammar from
// this package's internal test archive.
func EvaluateClosedForTest(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, inputs []valuedomain.Value) (heapdomain.Value, bool, bool) {
	return evaluateClosed(schema, values, operand, predecessor, inputs)
}
