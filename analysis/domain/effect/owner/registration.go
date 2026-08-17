package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/factor"
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
	EffectInput() *factor.Algebra
}

// AxisEntry is this package's effect axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A, *SchemaFragment, *HotOwner, factor.Value] {
	return axis.Spec[A, *SchemaFragment, *HotOwner, factor.Value]{
		Key:         "effect",
		Principal:   programartifact.RuleOutputEffect,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.EffectFactor },
		Declare: func(context axis.Declaration) (*SchemaFragment, bool) {
			return DeclareSchema(context.Builder, context.Bundle.EffectFactor)
		},
		Bind: func(context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
			return BindHot(context.Binding, context.Fragment, context.Inputs.EffectInput())
		},
		Algebra: func(owner *HotOwner) (axis.Algebra[factor.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[factor.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}
}
