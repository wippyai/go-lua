package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/static"
)

// MountRejection is this package's own evidence for a rejected pack mount. The
// composition carries it erased and a caller recovers it at this type, so the
// reason a pack mount rejected is stated by pack.
type MountRejection uint8

const (
	MountRejectionNone MountRejection = iota
	// MountRejectionInput is an absent Link, artifact view, or static
	// authority: the mount's own inputs are incomplete.
	MountRejectionInput
	// MountRejectionSeal is a complete input the pack seal rejected.
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
// and binds against: the Link the pack universe is derived from, the neutral
// mounted artifact view, the static authority the pack seal reads its mounted
// value substitutions from, and the pack schema itself once this axis has
// mounted it. It names only types this package already speaks, so the
// composition that supplies the input record satisfies it structurally and
// neither side learns the other's shape.
type axisInputs interface {
	LinkSource() *link.Link
	MountedArtifactCount() int
	MountedArtifactAt(index int) (axis.MountedArtifact, bool)
	StaticInput() *static.Authority
	PackInput() *pack.Schema
}

// mountPackSchema seals this Link's pack universe from the mounted artifacts.
// Each mount is qualified by its own module key, so two occurrences of one
// reusable Program artifact stay distinct coordinates and a repeated module key
// is rejected before the seal opens.
func mountPackSchema[A axisInputs](inputs A) (*pack.Schema, MountRejection, bool) {
	source := inputs.LinkSource()
	authority := inputs.StaticInput()
	count := inputs.MountedArtifactCount()
	if source == nil || authority == nil || count == 0 {
		return nil, MountRejectionInput, false
	}
	mounts := make([]pack.ArtifactMount, 0, count)
	seen := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		row, rowOK := inputs.MountedArtifactAt(index)
		if !rowOK {
			return nil, MountRejectionInput, false
		}
		mount, mountOK := pack.NewArtifactMount(row.Snapshot, row.ModuleKey, row.ProgramID)
		if !mountOK {
			return nil, MountRejectionInput, false
		}
		if _, duplicate := seen[mount.Module()]; duplicate {
			return nil, MountRejectionInput, false
		}
		seen[mount.Module()] = struct{}{}
		mounts = append(mounts, mount)
	}
	schema, sealed := pack.SealMountedArtifacts(source, authority, mounts)
	if !sealed || schema == nil {
		return nil, MountRejectionSeal, false
	}
	return schema, MountRejectionNone, true
}

// AxisEntry is this package's pack axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:         "pack",
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		// The pack factor's facts are published as one column, written by this
		// axis's own principal: the lane whose rules write the factor is the lane
		// the engine admits to fill the column a consumer reads it out of.
		Frame:    axis.Frame{Outputs: []axis.Output{{Key: "pack/facts", Writer: "pack"}}},
		Semantic: "semantic/factor/pack",
		Mount: axis.NewMount(func(context axis.Mounting[A]) (*pack.Schema, MountRejection, bool) {
			return mountPackSchema[A](context.Inputs)
		}),
	}
}

func DeclareAxis(builder *engine.SchemaBuilder, context axis.Declaration) (*SchemaFragment, bool) {
	semantic, ok := context.Roles.Key("semantic/factor/pack")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantic)
}

func BindAxis[A axisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
	return BindHot(binding, context.Fragment, context.Inputs.PackInput())
}

func AlgebraAxis(owner *HotOwner) (axis.Algebra[pack.Value], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[pack.Value]{}, false
	}
	return adoptFactor(spec)
}

func adoptFactor(spec engine.HotFactorSpec[coordinate, pack.Value]) (axis.Algebra[pack.Value], bool) {
	return axis.Adopt(axis.CarrierAlgebra[coordinate, pack.Value]{
		KeyEnd:      spec.KeyEnd,
		Lattice:     spec.Lattice,
		Default:     spec.Default,
		AdmitAt:     spec.AdmitAt,
		Fingerprint: spec.Fingerprint,
		Widen:       axis.CarrierRank[coordinate, pack.Value]{Width: spec.WidenRank.Width, At: spec.WidenRank.At},
		Narrow:      axis.CarrierRank[coordinate, pack.Value]{Width: spec.NarrowRank.Width, At: spec.NarrowRank.At},
	})
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the pack factor's own identity. A role is declared where it is
// used, so the row and the reference that names it are one package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("factor/pack")
}
