package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/pack"
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
	PackInput() *pack.Schema
}

// AxisEntry is this package's pack axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A, *SchemaFragment, *HotOwner, pack.Value] {
	return axis.Spec[A, *SchemaFragment, *HotOwner, pack.Value]{
		Key:         "pack",
		Principal:   programartifact.RuleOutputPack,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.PackFactor },
		Declare: func(context axis.Declaration) (*SchemaFragment, bool) {
			return DeclareSchema(context.Builder, context.Bundle.PackFactor)
		},
		Bind: func(context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
			return BindHot(context.Binding, context.Fragment, context.Inputs.PackInput())
		},
		Algebra: func(owner *HotOwner) (axis.Algebra[pack.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[pack.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}
}
