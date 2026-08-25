package closed

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// EvaluateClosedForTest is an internal-test-only bridge. It keeps the
// production evaluator unexported while the external semantic fixtures prove
// the exact successor without importing this package's grammar.
//
// It seals the SAME state the family binds - the judgment built from the two
// axis schemas - so the law observes the production quotient rather than a
// projection minted beside it.
func EvaluateClosedForTest(schema heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, inputs []valuedomain.Value) (heapdomain.Value, structure.ReductionOutcome) {
	judgment, judgmentOK := NewJudgment(schema, values)
	if !judgmentOK {
		return heapdomain.Value{}, structure.Refuse
	}
	return evaluateClosed(judgment.heaps, judgment.values, judgment.projection, operand, predecessor, inputs)
}
