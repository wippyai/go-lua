package closed

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// EvaluateClosedForTest is an internal-test-only bridge. It keeps the
// production evaluator unexported while the external semantic fixtures prove
// the exact successor without importing this package's grammar.
//
// It seals the SAME state the family binds - the judgment built from the two
// axis schemas and the selector projection - so the law observes the
// production quotient rather than one minted beside it.
//
// The projection is constructed here because these fixtures seal their two
// schemas directly and have no mount phase to receive one from. This function
// therefore stands in for that phase, which is why the seal law that admits
// exactly one construction site walks non-test declarations only.
func EvaluateClosedForTest(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, inputs []valuedomain.Value) (heapdomain.Value, structure.ReductionOutcome) {
	selectors, selectorsOK := keymatch.NewSelectorProjection(schema, values)
	if !selectorsOK {
		return heapdomain.Value{}, structure.Refuse
	}
	judgment, judgmentOK := NewJudgment(schema, values, selectors)
	if !judgmentOK {
		return heapdomain.Value{}, structure.Refuse
	}
	return evaluateClosed(judgment.heaps, judgment.values, judgment.projection, operand, predecessor, inputs)
}
