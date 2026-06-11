package factflow

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// ValueSourceKind classifies where a value-list slot comes from.
type ValueSourceKind uint8

const (
	ValueSourceUnknown ValueSourceKind = iota
	ValueSourceExpression
	ValueSourceCall
	ValueSourceVararg
	ValueSourceNil
)

// NoValueSourceIndex marks an index field that does not point at a source,
// target, or result slot.
const NoValueSourceIndex = -1

// ValueSource describes one value-list slot without retaining source AST.
type ValueSource struct {
	Kind ValueSourceKind

	ExprRef ExprRef
	HasExpr bool

	ExprIndex   int
	TargetIndex int
	ResultIndex int

	CallPoint    cfg.Point
	HasCallPoint bool

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

func copyValueSources(in []ValueSource) []ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]ValueSource, len(in))
	copy(out, in)
	return out
}
