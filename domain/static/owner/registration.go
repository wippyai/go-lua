// Package owner declares Static's Link-local static-type factor. The factor
// reuses Value's coordinate denominator while carrying only owner-fenced rows
// from Static's sealed Runtime relation.
package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const (
	AxisKey   schema.Key = "static-type"
	OutputKey schema.Key = "static-type/facts"
	AxisRole             = "factor/static-type"
)

// axisInputs states the complete owner inputs. Value supplies only the
// canonical coordinate denominator; Static supplies every fact and relation.
type axisInputs interface {
	StaticInput() *staticdomain.Authority
	ValueInput() *valuedomain.Schema
}

func AxisEntry[A axisInputs]() axis.Spec[A] {
	members := staticdomain.AxisMemberCatalog()
	return axis.Spec[A]{
		Key:          AxisKey,
		Storage:      axis.StorageFactor,
		Cardinality:  axis.CardinalityDense,
		Lifetime:     axis.LifetimeLink,
		Mutability:   axis.MutabilitySolve,
		Concurrency:  axis.ConcurrencySingleWriter,
		Dependencies: []schema.Key{"value"},
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: OutputKey, Writer: AxisKey}}},
		Catalog:      members,
		Signature:    axis.Signature{Key: staticdomain.CoordinateCarrier, Fact: staticdomain.TypeFactCarrier},
		Semantic:     "semantic/factor/static-type",
	}
}

func DeclareAxis(builder *engine.SchemaBuilder, declaration axis.Declaration) (*SchemaFragment, bool) {
	semantic, ok := declaration.Roles.Key("semantic/factor/static-type")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantic)
}

func BindAxis[A axisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
	owner, ownerOK := BindHot(binding, context.Fragment, context.Inputs.StaticInput(), context.Inputs.ValueInput())
	if !ownerOK || owner == nil || context.Fragment == nil {
		return nil, false
	}
	relationOwner := staticdomain.NewRelationOwner(context.Inputs.ValueInput())
	if !engine.BindRelationOwner(binding, context.Fragment.slot, relationOwner) {
		return nil, false
	}
	return owner, true
}

func AlgebraAxis(owner *HotOwner) (axis.Algebra[staticdomain.TypeFact], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[staticdomain.TypeFact]{}, false
	}
	return spec.AxisAlgebra()
}

func StructureSpecs() []structure.Spec { return vocabulary.RoleSpecs(AxisRole) }
