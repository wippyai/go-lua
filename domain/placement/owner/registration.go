package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// MountRejection is Placement's own cold mount evidence.
type MountRejection uint8

const (
	MountRejectionNone MountRejection = iota
	MountRejectionInput
)

// axisInputs states the one authority Placement projects. The composition
// supplies Heap; Placement does not reconstruct or duplicate its root index.
type axisInputs interface {
	HeapInput() heap.Schema
	PlacementInput() placement.Schema
}

// AxisEntry is Placement's factor-axis declaration.
func AxisEntry[A axisInputs]() axis.Spec[A] {
	members := placement.AxisMemberCatalog()
	return axis.Spec[A]{
		Key:          "placement",
		Storage:      axis.StorageFactor,
		Cardinality:  axis.CardinalityDense,
		Lifetime:     axis.LifetimeLink,
		Mutability:   axis.MutabilitySolve,
		Concurrency:  axis.ConcurrencySingleWriter,
		Dependencies: []schema.Key{"heap"},
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: "placement/facts", Writer: "placement"}}},
		Catalog:      members,
		Signature:    axis.Signature{Key: placement.PlacementKeyCarrier, Fact: placement.PlacementFactCarrier},
		Semantic:     "semantic/factor/placement",
		Roles:        []schema.Key{"semantic/factor/placement/summary-coordinatewise"},
		Mount: axis.NewMount(func(context axis.Mounting[A]) (placement.Schema, MountRejection, bool) {
			projected, ok := placement.NewSchema(context.Inputs.HeapInput())
			if !ok {
				return placement.Schema{}, MountRejectionInput, false
			}
			return projected, MountRejectionNone, true
		}),
	}
}

// DeclareAxis records Placement's exact Factor shape.
func DeclareAxis(builder *engine.SchemaBuilder, context axis.Declaration) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key("semantic/factor/placement")
	fold, foldOK := context.Roles.Key("semantic/factor/placement/summary-coordinatewise")
	if !semanticOK || !foldOK {
		return nil, false
	}
	fragment, ok := DeclareSchema(builder, semantic, fold)
	if !ok {
		return nil, false
	}
	return fragment, true
}

// BindAxis projects the same Heap authority into Placement's owner fence.
func BindAxis[A axisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
	schema := context.Inputs.PlacementInput()
	if !schema.Valid() || schema.Heap().ContentID() != context.Inputs.HeapInput().ContentID() {
		return nil, false
	}
	owner, ownerOK := BindHot(binding, context.Fragment, schema)
	if !ownerOK || owner == nil || context.Fragment == nil {
		return nil, false
	}
	// The generated Placement relation owner is installed once on the axis;
	// Store's foreign Value provider is resolved by sealed composition, never
	// copied into a per-rule helper.
	if !engine.BindRelationOwner(binding, context.Fragment.slot, placement.NewRelationOwner(schema)) {
		return nil, false
	}
	return owner, true
}

// AlgebraAxis publishes Placement's owner-typed factor algebra through the
// axis declaration surface.
func AlgebraAxis(owner *HotOwner) (axis.Algebra[placement.Fact], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[placement.Fact]{}, false
	}
	return spec.AxisAlgebra()
}

// StructureSpecs contributes Placement's factor identity to the semantic
// role vocabulary. Query roles are added with the query implementation.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("factor/placement", "factor/placement/summary-coordinatewise")
}
