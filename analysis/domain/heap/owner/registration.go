package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
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
	MountedArtifactAt(index int) (axis.MountedArtifact, bool)
	HeapInput() heap.Schema
}

// mountHeapSchema seals this Link's heap family from the mounted artifacts.
// Each mount is qualified by its own module key, so two occurrences of one
// reusable Program artifact stay distinct roots and a repeated module key is
// rejected before the seal opens.
func mountHeapSchema[A axisInputs](inputs A) (heap.Schema, heap.SealFailure, bool) {
	source := inputs.LinkSource()
	count := inputs.MountedArtifactCount()
	if source == nil || count == 0 {
		return heap.Schema{}, heap.SealFailureSource, false
	}
	mounts := make([]heap.ArtifactMount, 0, count)
	seen := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		row, rowOK := inputs.MountedArtifactAt(index)
		if !rowOK {
			return heap.Schema{}, heap.SealFailureProgramAllocations, false
		}
		mount, mountOK := heap.NewArtifactMount(row.Artifact, row.ModuleKey, row.ProgramID)
		if !mountOK {
			return heap.Schema{}, heap.SealFailureProgramAllocations, false
		}
		if _, duplicate := seen[mount.Module()]; duplicate {
			return heap.Schema{}, heap.SealFailureProgramAllocations, false
		}
		seen[mount.Module()] = struct{}{}
		mounts = append(mounts, mount)
	}
	schema, failure := heap.SealWithArtifacts(source, mounts)
	if failure != heap.SealFailureNone {
		return heap.Schema{}, failure, false
	}
	return schema, heap.SealFailureNone, true
}

// AxisEntry is this package's heap axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A, *SchemaFragment, *HotOwner, heap.Value] {
	return axis.Spec[A, *SchemaFragment, *HotOwner, heap.Value]{
		Key:         "heap",
		Principal:   programartifact.RuleOutputHeap,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.HeapFactor },
		Mount: axis.NewMount(func(context axis.Mounting[A]) (heap.Schema, heap.SealFailure, bool) {
			return mountHeapSchema[A](context.Inputs)
		}),
		Declare: func(context axis.Declaration) (*SchemaFragment, bool) {
			return DeclareSchema(context.Builder, context.Bundle.HeapFactor)
		},
		Bind: func(context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
			return BindHot(context.Binding, context.Fragment, context.Inputs.HeapInput())
		},
		Algebra: func(owner *HotOwner) (axis.Algebra[heap.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[heap.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}
}
