package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// axisInputs is this package's own statement of the Link input its axis mounts
// and binds against: the Link the value universe is derived from, the neutral
// mounted artifact view, the one peer authority the value schema is sealed
// over, and the value schema itself once this axis has mounted it. It names
// only types this package already speaks, so the composition that supplies the
// input record satisfies it structurally and neither side learns the other's
// shape.
type axisInputs interface {
	LinkSource() *link.Link
	MountedArtifactCount() int
	MountedArtifactAt(index int) (axis.MountedArtifact, bool)
	HeapInput() heap.Schema
	ValueInput() *value.Schema
}

// mountValueSchema seals this Link's value universe from the mounted artifacts.
// Each mount is qualified by its own module key, so two occurrences of one
// reusable Program artifact stay distinct coordinates and a repeated module key
// is rejected before the seal opens.
func mountValueSchema[A axisInputs](inputs A) (*value.Schema, value.SealFailure, bool) {
	source := inputs.LinkSource()
	count := inputs.MountedArtifactCount()
	if source == nil || count == 0 {
		return nil, value.SealFailureInput, false
	}
	mounts := make([]value.ArtifactMount, 0, count)
	seen := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		row, rowOK := inputs.MountedArtifactAt(index)
		if !rowOK {
			return nil, value.SealFailureInput, false
		}
		mount, mountOK := value.NewArtifactMount(row.Artifact, row.ModuleKey, row.ProgramID)
		if !mountOK {
			return nil, value.SealFailureInput, false
		}
		if _, duplicate := seen[mount.Module()]; duplicate {
			return nil, value.SealFailureInput, false
		}
		seen[mount.Module()] = struct{}{}
		mounts = append(mounts, mount)
	}
	schema, failure := value.SealWithFailure(source, inputs.HeapInput(), mounts)
	if failure != value.SealFailureNone {
		return nil, failure, false
	}
	return schema, value.SealFailureNone, true
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
		// The value universe is sealed over the heap family, so the heap axis
		// is this axis's declared dependency and its authority is present
		// before this mount opens.
		Dependencies: []schema.Key{"heap"},
		Semantic:     func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.ValueFactor },
		Mount: axis.NewMount(func(context axis.Mounting[A]) (*value.Schema, value.SealFailure, bool) {
			return mountValueSchema[A](context.Inputs)
		}),
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
