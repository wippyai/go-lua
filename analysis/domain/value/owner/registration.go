package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// axisInputs is this package's own statement of the Link input its axis binds
// against. It names only a peer type this package already speaks, so the
// composition that supplies the input record satisfies it structurally and
// neither side learns the other's shape.
type axisInputs interface {
	ValueInput() *value.Schema
}

// AxisEntry is this package's value axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A, *SchemaFragment, *HotOwner, value.Value] {
	return axis.Spec[A, *SchemaFragment, *HotOwner, value.Value]{
		Key:         "value",
		Principal:   programartifact.RuleOutputValue,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.ValueFactor },
		Declare: func(context axis.Declaration) (*SchemaFragment, bool) {
			return DeclareSchema(context.Builder, context.Bundle.ValueFactor, context.Bundle.ValueSummary, context.Bundle.ValueSummaryFold)
		},
		Bind: func(context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
			return BindHot(context.Binding, context.Fragment, context.Inputs.ValueInput())
		},
		Algebra: func(owner *HotOwner) (axis.Algebra[value.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[value.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}
}
