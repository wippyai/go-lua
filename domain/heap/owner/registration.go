package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/heap"
)

// axisInputs is this package's own statement of the Link input its axis mounts
// and binds against: the Link the heap family is derived from, the neutral
// mounted artifact view, and the heap schema itself once this axis has mounted
// it. It names only types this package already speaks, so the composition that
// supplies the input record satisfies it structurally and neither side learns
// the other's shape.
type axisInputs interface {
	LinkSource() *link.Link
	MountedArtifactCount() int
	MountedArtifactAt(index int) (programmount.MountedArtifact, bool)
	HeapInput() heap.Schema
}

// mountHeapSchema seals this Link's heap family from the mounted artifacts.
// Each mount is qualified by its own module key, so two occurrences of one
// reusable Program artifact stay distinct roots and a repeated module key is
// rejected before the seal opens.
func mountHeapSchema[A axisInputs](inputs A) (heap.Schema, heap.SealFailure, bool) {
	source := inputs.LinkSource()
	count := inputs.MountedArtifactCount()
	if count == 0 {
		return heap.Schema{}, heap.SealFailureSource, false
	}
	// The artifact view is read first so a row this domain cannot place is
	// reported as the mount it is, and an absent Link stays the source verdict.
	mounts := make([]programmount.MountedArtifact, 0, count)
	seen := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		row, rowOK := inputs.MountedArtifactAt(index)
		if !rowOK {
			return heap.Schema{}, heap.SealFailureProgramAllocations, false
		}
		if !row.Available() {
			return heap.Schema{}, heap.SealFailureProgramAllocations, false
		}
		if _, duplicate := seen[row.ModuleKey]; duplicate {
			return heap.Schema{}, heap.SealFailureProgramAllocations, false
		}
		seen[row.ModuleKey] = struct{}{}
		mounts = append(mounts, row)
	}
	if source == nil {
		return heap.Schema{}, heap.SealFailureSource, false
	}
	schema, failure := heap.SealWithArtifacts(source, mounts)
	if failure != heap.SealFailureNone {
		return heap.Schema{}, failure, false
	}
	return schema, heap.SealFailureNone, true
}

// AxisEntry is this package's heap axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A] {
	members := heap.AxisMemberCatalog()
	return axis.Spec[A]{
		Key:         "heap",
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		// The heap factor's facts are published as one column, written by this
		// axis's own principal: the lane whose rules write the factor is the lane
		// the engine admits to fill the column a consumer reads it out of.
		Frame:     axis.Frame{Outputs: []axis.Output{{Key: "heap/facts", Writer: "heap"}}},
		Catalog:   members,
		Signature: axis.Signature{Key: heap.HeapKeyCarrier, Fact: heap.HeapFactCarrier},
		Semantic:  "semantic/factor/heap",
		Roles:     []schema.Key{"semantic/factor/heap/summary-complete"},
		Mount: axis.NewMount(func(context axis.Mounting[A]) (heap.Schema, heap.SealFailure, bool) {
			return mountHeapSchema[A](context.Inputs)
		}),
	}
}

func DeclareAxis(builder *engine.SchemaBuilder, context axis.Declaration) (*SchemaFragment, bool) {
	semantic, ok := context.Roles.Key("semantic/factor/heap")
	summary, summaryOK := context.Roles.Key("semantic/factor/heap/summary-complete")
	if !ok || !summaryOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, summary)
}

func BindAxis[A axisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
	owner, ownerOK := BindHot(binding, context.Fragment, context.Inputs.HeapInput())
	if !ownerOK || owner == nil || context.Fragment == nil {
		return nil, false
	}
	if !engine.BindRelationOwner(binding, context.Fragment.slot, heap.NewRelationOwner(context.Inputs.HeapInput())) {
		return nil, false
	}
	return owner, true
}

func AlgebraAxis(owner *HotOwner) (axis.Algebra[heap.Value], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[heap.Value]{}, false
	}
	return spec.AxisAlgebra()
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the heap factor's own identity and complete-vector summary
// form. A role is declared where it is used, so the row and the reference that
// names it are one package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("factor/heap", "factor/heap/summary-complete")
}
