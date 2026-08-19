package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/call"
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
		mounts = append(mounts, call.MountedArtifact{ModuleKey: row.ModuleKey, Snapshot: row.Snapshot})
	}
	algebra, sealed := call.NewWithMountedArtifacts(source, mounts)
	if !sealed || algebra == nil || !algebra.Valid() {
		return nil, MountRejectionSeal, false
	}
	return algebra, MountRejectionNone, true
}

// AxisEntry is this package's call axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:         "call",
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		// The call factor's facts are published as one column, written by this
		// axis's own principal: the lane whose rules write the factor is the lane
		// the engine admits to fill the column a consumer reads it out of.
		Frame:    axis.Frame{Outputs: []axis.Output{{Key: "call/facts", Writer: "call"}}},
		Semantic: "semantic/factor/call",
		Mount: axis.NewMount(func(context axis.Mounting[A]) (*call.Algebra, MountRejection, bool) {
			return mountCallAlgebra[A](context.Inputs)
		}),
	}
}

func DeclareAxis(builder *engine.SchemaBuilder, context axis.Declaration) (*SchemaFragment, bool) {
	semantic, ok := context.Roles.Key("semantic/factor/call")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantic)
}

func BindAxis[A axisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
	return BindHot(binding, context.Fragment, context.Inputs.CallInput())
}

func AlgebraAxis(owner *HotOwner) (axis.Algebra[call.Value], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[call.Value]{}, false
	}
	return adoptFactor(spec)
}

func adoptFactor(spec engine.HotFactorSpec[coordinate, call.Value]) (axis.Algebra[call.Value], bool) {
	return axis.Adopt(axis.CarrierAlgebra[coordinate, call.Value]{
		KeyEnd:      spec.KeyEnd,
		Lattice:     spec.Lattice,
		Default:     spec.Default,
		AdmitAt:     spec.AdmitAt,
		Fingerprint: spec.Fingerprint,
		Widen:       axis.CarrierRank[coordinate, call.Value]{Width: spec.WidenRank.Width, At: spec.WidenRank.At},
		Narrow:      axis.CarrierRank[coordinate, call.Value]{Width: spec.NarrowRank.Width, At: spec.NarrowRank.At},
	})
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the call factor's own identity. A role is declared where it is
// used, so the row and the reference that names it are one package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("factor/call")
}
