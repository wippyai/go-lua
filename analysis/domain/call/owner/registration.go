package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/call"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// MountRejection is this package's own evidence for a rejected call mount. The
// composition carries it erased and a caller recovers it at this type, so the
// reason a call mount rejected is stated by call.
type MountRejection uint8

const (
	MountRejectionNone MountRejection = iota
	// MountRejectionInput is an absent Link or artifact view: the mount's own
	// inputs are incomplete.
	MountRejectionInput
	// MountRejectionSeal is a complete input the call algebra rejected.
	MountRejectionSeal
)

func (rejection MountRejection) String() string {
	switch rejection {
	case MountRejectionInput:
		return "input"
	case MountRejectionSeal:
		return "seal"
	default:
		return "none"
	}
}

// axisInputs is this package's own statement of the Link input its axis mounts
// and binds against: the Link the call algebra is derived from, the neutral
// mounted artifact view, and the algebra itself once this axis has mounted it.
// It names only types this package already speaks, so the composition that
// supplies the input record satisfies it structurally and neither side learns
// the other's shape.
type axisInputs interface {
	LinkSource() *link.Link
	MountedArtifactCount() int
	MountedArtifactAt(index int) (axis.MountedArtifact, bool)
	CallInput() *call.Algebra
}

// mountCallAlgebra seals this Link's call algebra from the mounted artifacts.
// Call enumerates and validates every target row itself; this mount supplies
// the artifact placed at each module key and nothing else.
func mountCallAlgebra[A axisInputs](inputs A) (*call.Algebra, MountRejection, bool) {
	source := inputs.LinkSource()
	count := inputs.MountedArtifactCount()
	if source == nil || count == 0 {
		return nil, MountRejectionInput, false
	}
	mounts := make([]call.MountedArtifact, 0, count)
	seen := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		row, rowOK := inputs.MountedArtifactAt(index)
		if !rowOK {
			return nil, MountRejectionInput, false
		}
		if _, duplicate := seen[row.ModuleKey]; duplicate {
			return nil, MountRejectionInput, false
		}
		seen[row.ModuleKey] = struct{}{}
		mounts = append(mounts, call.MountedArtifact{ModuleKey: row.ModuleKey, Artifact: row.Artifact})
	}
	algebra, sealed := call.NewWithMountedArtifacts(source, mounts)
	if !sealed || algebra == nil || !algebra.Valid() {
		return nil, MountRejectionSeal, false
	}
	return algebra, MountRejectionNone, true
}

// AxisEntry is this package's call axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A, *SchemaFragment, *HotOwner, call.Value] {
	return axis.Spec[A, *SchemaFragment, *HotOwner, call.Value]{
		Key:         "call",
		Principal:   programartifact.RuleOutputCall,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.CallFactor },
		Mount: axis.NewMount(func(context axis.Mounting[A]) (*call.Algebra, MountRejection, bool) {
			return mountCallAlgebra[A](context.Inputs)
		}),
		Declare: func(context axis.Declaration) (*SchemaFragment, bool) {
			return DeclareSchema(context.Builder, context.Bundle.CallFactor)
		},
		Bind: func(context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
			return BindHot(context.Binding, context.Fragment, context.Inputs.CallInput())
		},
		Algebra: func(owner *HotOwner) (axis.Algebra[call.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[call.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}
}
