// Package probe is the declaration-surface conformance fixture of the analyzer.
// It is a domain in everything the declaration table can see: it declares its
// own coordinate space, the rule that writes it, the semantic roles both are
// identified by, the observation population its finding is measured over, and a
// published diagnostic under a publication family no other domain declares. All
// of it is declared here, from this package alone, naming the declaration
// surfaces and nothing else, and it is composed into a table beside the
// analyzer's own rows by the walk law that consumes this package.
//
// # What this fixture states, and what it does not
//
// It states that the surfaces can spell a whole new domain. It does not state
// that the engine can run one. The hook sets a lane declares are present because
// the surfaces admit a row against the hook set its lane requires, and every hook
// except the mount rejects: instantiating a factor binding, registering an engine
// rule slot, and being reached by a compiled artifact's occurrence rows are the
// artifact and engine half, and a fixture that returned a usable zero value from
// those would claim a capability nothing here has proven. A probe that ever
// reaches a live composition therefore fails at its first executable step rather
// than binding as an empty rule.
//
// The mount hook is real. Sealing a Link authority from the mounted artifact view
// needs no engine binding, so this fixture seals one from the neutral view alone
// and the walk runs it.
//
// # Not an analyzer domain
//
// Nothing in the analyzer's own composition names this package, and the law
// beside it states that it names no analyzer package outside the declaration
// surfaces and the neutral engine vocabulary its hook signatures are typed in.
package probe

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The identities this domain declares. Each is authored here and named from
// here, so the rows and the references that resolve them are one package's
// statement.
const (
	// AxisKey is this domain's coordinate space, and so its writer principal:
	// the entry identity it derives is the principal identity, and its
	// declaration position is the principal slot.
	AxisKey schema.Key = "probe"
	// OutputKey is the one column this axis publishes.
	OutputKey schema.Key = "probe/coordinates"
	// RuleKey is the rule that writes the axis.
	RuleKey schema.Key = "probe-source"
	// FamilyKey is the publication family this domain's finding is gated by. No
	// other domain declares it, and no enum in the analyzer holds it.
	FamilyKey schema.Key = "family/probe"
	// ObservationKey is the population this domain's finding is measured over.
	ObservationKey schema.Key = "observation/probe-population"
	// Code is this domain's published finding. Its first segment is the declared
	// spelling of FamilyKey, which is the whole of what the family law states.
	Code diagnostic.Code = "probe.example"
)

// The semantic roles this domain owns: the identity its axis binds its factor
// under, and the rule/operand forms its rule is identified by.
const (
	FactorRole  = "factor/probe"
	RuleRole    = "rule/probe/source"
	OperandRole = "operand/probe/source"
)

// StructureSpecs is this domain's contribution to the structural vocabulary:
// the three semantic roles it is identified by, the publication family its code
// is gated by, and the observation population its finding is measured over.
//
// The family and the observation carry no authored ordinal. No foreign spelling
// numbers either vocabulary at a position this domain could claim, so the
// aggregation numbers them and a consumer resolves them by key.
func StructureSpecs() []structure.Spec {
	specs := append(vocabulary.RoleSpecs(FactorRole), vocabulary.RuleRoleSpecs("probe/source")...)
	return append(specs,
		structure.Spec{Key: FamilyKey, Category: structure.CategoryDiagnosticFamily, Spelling: "probe", Accepted: true},
		structure.Spec{Key: ObservationKey, Category: structure.CategoryDiagnosticObservation, Spelling: "probe-population", Accepted: true},
	)
}

// mountInputs is this package's own statement of what its mount hook reads from
// the composition's Link input record: the size of the neutral mounted artifact
// view. It names no composition type, so any record that carries that view
// satisfies it structurally and neither side learns the other's shape.
type mountInputs interface {
	MountedArtifactCount() int
}

// MountAuthority is the Link authority this domain seals for itself: the number
// of artifacts mounted in the phase that sealed it.
type MountAuthority struct{ Artifacts int }

// MountRejection is this domain's own rejection evidence. It travels back to the
// composition erased and is recovered at this type by the caller.
type MountRejection struct{ Artifacts int }

// SchemaFragment and HotAxis are this axis's cold and hot halves. They are
// declared so the surface's typed hooks have the shape they are instantiated
// against; neither is ever produced, because this fixture declares no executable
// half.
type (
	SchemaFragment struct{}
	HotAxis        struct{}
)

// AxisEntry is this domain's axis declaration. A is the composition's own Link
// input record, admitted by the need interface above.
func AxisEntry[A mountInputs]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:         AxisKey,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalitySparse,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: OutputKey, Writer: AxisKey}}},
		Semantic:    vocabulary.RoleKey(FactorRole),
		Mount: axis.NewMount(func(context axis.Mounting[A]) (MountAuthority, MountRejection, bool) {
			count := context.Inputs.MountedArtifactCount()
			if count <= 0 {
				return MountAuthority{}, MountRejection{Artifacts: count}, false
			}
			return MountAuthority{Artifacts: count}, MountRejection{}, true
		}),
	}
}

// RuleFragment and HotRule are this rule's cold and hot halves, declared for the
// same reason and produced for the same reason: never.
type (
	RuleFragment struct{}
	HotRule      struct{}
)

// RuleEntry is this domain's rule declaration. It writes the axis this package
// declares, and it subscribes to the point-attachment occurrence family, which
// is the one family every compiled program carries. P and A are the composition's
// principal and authority records; this rule declares against neither, because
// the only principal it writes is its own.
func RuleEntry[P, A any]() rule.Spec {
	return rule.Spec{
		Key:    RuleKey,
		Lane:   rule.LaneMounted,
		Writes: AxisKey,
		Owner:  AxisKey,
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/point-attachment", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none"},
		},
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
	}
}

// DiagnosticSpec is this domain's published finding: one static row, gated by
// the family this package declares and measured over the population it declares
// beside it.
func DiagnosticSpec() diagnostic.Spec {
	return diagnostic.Spec{
		Code:            Code,
		Family:          diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: FamilyKey},
		DefaultSeverity: diagnostic.SeverityHint,
		Lane:            diagnostic.LaneStatic,
		Observation:     diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: ObservationKey},
		Requirements:    diagnostic.RequiresSubject,
		Message:         "probe population carries {subject}",
		Help:            "This row is a fixture: it publishes under a family declared outside the analyzer's own inventory.",
		Evidence: []diagnostic.Evidence{{
			Anchor: diagnostic.AnchorPrimary,
			Kind:   "declared row",
			Trust:  "declared",
			Reason: "unspecified",
			Detail: "the probe population names {subject}",
		}},
		Labels: []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "probe subject"}},
		Render: []diagnostic.Section{
			diagnostic.SectionSummary,
			diagnostic.SectionLocation,
			diagnostic.SectionSource,
			diagnostic.SectionEvidence,
			diagnostic.SectionHelp,
		},
	}
}
