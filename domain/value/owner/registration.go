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
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/value"
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
	MountedArtifactAt(index int) (programmount.MountedArtifact, bool)
	HeapInput() heap.Schema
	CallInput() *calldomain.Algebra
	ValueInput() *value.Schema
	StructureInput() structure.Table
}

// mountValueSchema seals this Link's value universe from the mounted artifacts.
// Each mount is qualified by its own module key, so two occurrences of one
// reusable Program artifact stay distinct coordinates and a repeated module key
// is rejected before the seal opens. Call's sealed algebra is the peer
// authority a call-result candidate row copies its mounted-call coordinate
// from, so it is supplied to the seal rather than reopened by a hot rule.
func mountValueSchema[A axisInputs](inputs A) (*value.Schema, value.SealFailure, bool) {
	source := inputs.LinkSource()
	count := inputs.MountedArtifactCount()
	if source == nil || count == 0 {
		return nil, value.SealFailureInput, false
	}
	mounts := make([]programmount.MountedArtifact, 0, count)
	seen := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		row, rowOK := inputs.MountedArtifactAt(index)
		if !rowOK {
			return nil, value.SealFailureInput, false
		}
		if !row.Available() {
			return nil, value.SealFailureInput, false
		}
		if _, duplicate := seen[row.ModuleKey]; duplicate {
			return nil, value.SealFailureInput, false
		}
		seen[row.ModuleKey] = struct{}{}
		mounts = append(mounts, row)
	}
	schema, failure := value.SealWithFailure(source, inputs.HeapInput(), inputs.CallInput(), mounts, inputs.StructureInput())
	if failure != value.SealFailureNone {
		return nil, failure, false
	}
	return schema, value.SealFailureNone, true
}

// AxisEntry is this package's value axis declaration. A is the composition's
// own Link input record, admitted by the need interface above.
func AxisEntry[A axisInputs]() axis.Spec[A] {
	members := value.AxisMemberCatalog()
	return axis.Spec[A]{
		Key:         "value",
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		// The value universe is sealed over the heap family and over Call's
		// mounted-call coordinate projection, so both are this axis's declared
		// dependencies and both authorities are present before this mount
		// opens.
		Dependencies: []schema.Key{"heap", "call"},
		// The value factor's facts are published as one column, written by this
		// axis's own principal: the lane whose rules write the factor is the
		// lane the engine admits to fill the column a consumer reads it out of.
		Frame:     axis.Frame{Outputs: []axis.Output{{Key: "value/facts", Writer: "value"}}},
		Catalog:   members,
		Signature: axis.Signature{Key: value.ValueCoordinateCarrier, Fact: value.ValueFactCarrier},
		Semantic:  "semantic/factor/value",
		Roles:     []schema.Key{"semantic/factor/value/summary-identity", "semantic/factor/value/summary-coordinatewise"},
		Mount: axis.NewMount(func(context axis.Mounting[A]) (*value.Schema, value.SealFailure, bool) {
			return mountValueSchema[A](context.Inputs)
		}),
	}
}

// DeclareAxis records the value factor's cold schema shape.
func DeclareAxis(builder *engine.SchemaBuilder, context axis.Declaration) (*SchemaFragment, bool) {
	factor, factorOK := context.Roles.Key("semantic/factor/value")
	summary, summaryOK := context.Roles.Key("semantic/factor/value/summary-identity")
	fold, foldOK := context.Roles.Key("semantic/factor/value/summary-coordinatewise")
	if !factorOK || !summaryOK || !foldOK {
		return nil, false
	}
	return DeclareSchema(builder, factor, summary, fold)
}

// BindAxis instantiates the value factor binding.
func BindAxis[A axisInputs](binding *engine.SchemaBinding, context axis.Binding[A, *SchemaFragment]) (*HotOwner, bool) {
	owner, ownerOK := BindHot(binding, context.Fragment, context.Inputs.ValueInput())
	if !ownerOK || owner == nil || context.Fragment == nil {
		return nil, false
	}
	// Generated Rules resolve Value's member relations through the one axis
	// owner. Install it exactly once on the Value Factor; no Rule receives a
	// per-rule relation provider.
	relationOwner := value.NewRelationOwner(context.Inputs.ValueInput())
	if !engine.BindRelationOwner(binding, context.Fragment.slot, relationOwner) {
		return nil, false
	}
	return owner, true
}

// AlgebraAxis publishes the value factor algebra on the axis ordinal key space.
func AlgebraAxis(owner *HotOwner) (axis.Algebra[value.Value], bool) {
	spec, ok := owner.FactorSpec()
	if !ok {
		return axis.Algebra[value.Value]{}, false
	}
	return spec.AxisAlgebra()
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the value factor's own identity, the two summary forms its
// schema is declared with, and the query family its facts are published
// through. A role is declared where it is used, so the row and the reference
// that names it are one package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(
		"factor/value",
		"factor/value/summary-identity",
		"factor/value/summary-coordinatewise",
		"query/value-summary",
		"query-result/value-summary",
	)
}
