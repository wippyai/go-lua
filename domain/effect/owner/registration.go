package owner

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/pack"
)

// MountRejection is this package's own evidence for a rejected effect mount.
// The composition carries it erased and a caller recovers it at this type, so
// the reason an effect mount rejected is stated by effect.
type MountRejection uint8

const (
	MountRejectionNone MountRejection = iota
	// MountRejectionInput is an absent Link, artifact view, or pack authority:
	// the mount's own inputs are incomplete.
	MountRejectionInput
	// MountRejectionContract is a Link whose boundary publishes no target
	// contract, so the effect vocabulary has no operations to seal against.
	MountRejectionContract
	// MountRejectionSeal is a complete input the effect algebra rejected.
	MountRejectionSeal
)

func (rejection MountRejection) String() string {
	switch rejection {
	case MountRejectionInput:
		return "input"
	case MountRejectionContract:
		return "contract"
	case MountRejectionSeal:
		return "seal"
	default:
		return "none"
	}
}

// axisInputs is this package's own statement of the Link input its axis mounts
// and binds against: the Link the effect algebra is derived from, the neutral
// mounted artifact view, the one peer authority the algebra is sealed over, and
// the algebra itself once this axis has mounted it. The target contract is not
// among them: it is the Link's own boundary term, read here from the Link
// rather than carried a second time. It names only types this package already
// speaks, so the composition that supplies the input record satisfies it
// structurally and neither side learns the other's shape.
type axisInputs interface {
	LinkSource() *link.Link
	MountedArtifactCount() int
	MountedArtifactAt(index int) (axis.MountedArtifact, bool)
	PackInput() *pack.Schema
	EffectInput() *factor.Algebra
}

// mountEffectAlgebra seals this Link's effect algebra from the mounted
// artifacts. Effect enumerates and validates every body row itself; this mount
// supplies the artifact placed at each module key, the pack authority the
// algebra is sealed over, and the Link's own target contract.
func mountEffectAlgebra[A axisInputs](inputs A) (*factor.Algebra, MountRejection, bool) {
	source := inputs.LinkSource()
	packs := inputs.PackInput()
	count := inputs.MountedArtifactCount()
	if source == nil || packs == nil || count == 0 {
		return nil, MountRejectionInput, false
	}
	if source.Boundary() == nil {
		return nil, MountRejectionContract, false
	}
	contract, contractOK := source.Boundary().Target()
	if !contractOK || contract == nil {
		return nil, MountRejectionContract, false
	}
	mounts := make([]factor.MountedArtifact, 0, count)
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
		mounts = append(mounts, factor.MountedArtifact{ModuleKey: row.ModuleKey, Artifact: row.Artifact})
	}
	algebra, sealed := factor.NewWithMountedArtifacts(source, packs, contract, mounts)
	if !sealed || algebra == nil || !algebra.Valid() {
		return nil, MountRejectionSeal, false
	}
	return algebra, MountRejectionNone, true
}

// AxisEntry is this package's effect axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A, *SchemaFragment, *HotOwner, factor.Value] {
	return axis.Spec[A, *SchemaFragment, *HotOwner, factor.Value]{
		Key:         "effect",
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		// The effect algebra is sealed over the pack universe, so the pack axis
		// is this axis's declared dependency and its authority is present
		// before this mount opens.
		Dependencies: []schema.Key{"pack"},
		// The effect factor's facts are published as one column, written by this
		// axis's own principal: the lane whose rules write the factor is the lane
		// the engine admits to fill the column a consumer reads it out of.
		Frame:    axis.Frame{Outputs: []axis.Output{{Key: "effect/facts", Writer: "effect"}}},
		Semantic: "semantic/factor/effect",
		Mount: axis.NewMount(func(context axis.Mounting[A]) (*factor.Algebra, MountRejection, bool) {
			return mountEffectAlgebra[A](context.Inputs)
		}),
		Declare: func(context axis.Declaration) (*SchemaFragment, bool) {
			semantic, ok := context.Roles.Key("semantic/factor/effect")
			if !ok {
				return nil, false
			}
			return DeclareSchema(context.Builder, semantic)
		},
		Bind: func(context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
			return BindHot(context.Binding, context.Fragment, context.Inputs.EffectInput())
		},
		Algebra: func(owner *HotOwner) (axis.Algebra[factor.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[factor.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the effect factor's own identity and the query family its
// facts are published through. A role is declared where it is used, so the row
// and the reference that names it are one package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(
		"factor/effect",
		"query/effect-exact",
		"query-result/effect-exact",
	)
}
