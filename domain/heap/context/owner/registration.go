package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
)

type axisInputs interface {
	ContextInput() contextdomain.Schema
}

// AxisEntry is Context's allocation-key factor declaration. The Context
// authority is derived by the composition from the exact Heap and Link
// ContextDirectory, so this axis has no caller-supplied mount hook.
func AxisEntry[A axisInputs]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:          "context",
		Storage:      axis.StorageFactor,
		Cardinality:  axis.CardinalityDense,
		Lifetime:     axis.LifetimeLink,
		Mutability:   axis.MutabilitySolve,
		Concurrency:  axis.ConcurrencySingleWriter,
		Dependencies: []schema.Key{"heap"},
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: "context/facts", Writer: "context"}}},
		Semantic:     "semantic/factor/context",
	}
}

func DeclareAxis(builder *engine.SchemaBuilder, declaration axis.Declaration) (*SchemaFragment, bool) {
	semantic, ok := declaration.Roles.Key("semantic/factor/context")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantic)
}

func BindAxis[A axisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
	return BindHot(binding, context.Fragment, context.Inputs.ContextInput())
}

func AlgebraAxis(owner *HotOwner) (axis.Algebra[contextdomain.Value], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[contextdomain.Value]{}, false
	}
	return spec.AxisAlgebra()
}

// StructureSpecs contributes Context's semantic identity to the composition
// vocabulary. The domain owns the spelling; the composition only aggregates it.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("factor/context")
}
