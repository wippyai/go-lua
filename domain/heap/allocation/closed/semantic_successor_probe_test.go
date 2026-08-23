package closed

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// EvaluateClosedForTest is an internal-test-only bridge. It keeps the
// production evaluator unexported while allowing the external receipt fixture
// to prove the exact semantic successor without importing grammar from
// this package's internal test archive. It seals the same key/class
// projection BindHot seals, so the law observes the production quotient.
func EvaluateClosedForTest(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, inputs []valuedomain.Value) (heapdomain.Value, structure.ReductionOutcome) {
	projection, projectionOK := keymatch.NewSelectorProjection(schema, values)
	if !projectionOK {
		return heapdomain.Value{}, structure.Refuse
	}
	return evaluateClosed(schema, values, projection, operand, predecessor, inputs)
}
