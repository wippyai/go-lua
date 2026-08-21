package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// BindStage names the phase of the one binding transaction that rejected. A
// per-rule phase additionally names the rule and its own pass.
type BindStage uint8

const (
	BindStageNone BindStage = iota
	BindStageInput
	BindStageTable
	BindStageCompilation
	BindStageBinding
	BindStagePrincipal
	BindStageAllocationCatalog
	BindStageRule
	BindStageQueries
	BindStagePublication
	BindStageSeal
	BindStageAllocations
)

func (stage BindStage) String() string {
	switch stage {
	case BindStageInput:
		return "input"
	case BindStageTable:
		return "table"
	case BindStageCompilation:
		return "compilation"
	case BindStageBinding:
		return "binding"
	case BindStagePrincipal:
		return "principal"
	case BindStageAllocationCatalog:
		return "allocation-catalog"
	case BindStageRule:
		return "rule"
	case BindStageQueries:
		return "queries"
	case BindStagePublication:
		return "publication"
	case BindStageSeal:
		return "seal"
	case BindStageAllocations:
		return "allocations"
	default:
		return "none"
	}
}

// BindFailure is the closed verdict of one rejected binding transaction. It
// names the phase, for a per-rule phase the exact rule and pass, and for the
// axis phase the exact axis; no schema slot, callback, coordinate, or mutable
// binding state escapes with it.
type BindFailure struct {
	Stage      BindStage
	Rule       DiagnosticRule
	RuleStage  RuleBindStage
	Axis       DiagnosticAxis
	Allocation allocationcatalog.SealFailure
}

func (failure BindFailure) Available() bool { return failure.Stage != BindStageNone }

func (failure BindFailure) String() string {
	if !failure.Available() {
		return "none"
	}
	if failure.Stage == BindStageRule {
		return failure.Rule.String() + "/" + failure.RuleStage.String()
	}
	if failure.Stage == BindStagePrincipal && failure.Axis != DiagnosticAxisUnknown {
		return failure.Stage.String() + "/" + failure.Axis.String()
	}
	return failure.Stage.String()
}

// catalogBinding is the sealed Link-local binding of the whole catalog: the
// sealed engine binding, the rule table's hot projection, the allocation
// catalog the Link owns, and every declared query family's sealed
// implementation. Each cell is opaque here and is recovered at its type by the
// family that declared it.
type catalogBinding struct {
	catalog     *catalog
	binding     *engine.SchemaBinding
	rules       *RuleBinding
	allocations *allocationcatalog.Catalog
	value       *valueowner.HotOwner
	queries     queryCells
}

func (bound catalogBinding) available() bool {
	return bound.binding != nil && bound.binding.Sealed() && bound.rules != nil && bound.allocations != nil &&
		bound.value != nil && bound.catalog != nil && bound.queries.available(bound.catalog.queries)
}

// bind is the one hot binding transaction for one compilation-owned catalog. It
// binds the factor principals, opens the Link allocation catalog, drives the
// whole rule table, binds every declared query family against the principals
// that pass produced, seals, and recovers each family's implementation.
//
// Every plane's sequence is a pass over the sealed table. The caller supplies
// the mount phase's own record and receives back only the neutral execution
// surface.
func bind(compilation Compilation, inputs LinkInputs) (catalogBinding, BindFailure) {
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		return catalogBinding{}, BindFailure{Stage: BindStageTable}
	}
	if !inputs.available() {
		return catalogBinding{}, BindFailure{Stage: BindStageInput}
	}
	if !compilation.Available() || !compilation.catalog.axisFragments.available(state.axes) {
		return catalogBinding{}, BindFailure{Stage: BindStageCompilation}
	}
	binding := engine.NewSchemaBindingForExecution(compilation.Schema(), compilation.ExecutionSchemaID())
	if binding == nil {
		return catalogBinding{}, BindFailure{Stage: BindStageBinding}
	}
	// The axis pass is first: every factor principal is bound by its own
	// declared axis, in the sealed table's order, before any rule or catalog
	// consumes one.
	axes, failedAxis, axesOK := bindAxes(state, binding, compilation.catalog.axisFragments, inputs)
	if !axesOK {
		return catalogBinding{}, BindFailure{Stage: BindStagePrincipal, Axis: failedAxis}
	}
	value, valueOK := axisPayloadForKey[*valueowner.HotOwner](state, axes, axisKeyValue)
	if !valueOK {
		return catalogBinding{}, BindFailure{Stage: BindStagePrincipal}
	}
	// The mount set the allocation catalog joins is the heap schema's own: it
	// sealed those mounts, so the list is read back from it rather than carried
	// beside it as a second copy.
	allocations, allocationFailure := allocationcatalog.BeginWithFailure(inputs.HeapSchema, inputs.ValueSchema, inputs.HeapSchema.ArtifactMounts())
	if allocationFailure != allocationcatalog.SealFailureNone {
		return catalogBinding{}, BindFailure{Stage: BindStageAllocationCatalog, Allocation: allocationFailure}
	}
	set, setOK := axes.hotPrincipals(state, inputs, allocations)
	if !setOK {
		return catalogBinding{}, BindFailure{Stage: BindStagePrincipal}
	}
	fragments := compilation.catalog.queryFragments
	if !fragments.available(state.queries) {
		return catalogBinding{}, BindFailure{Stage: BindStageQueries}
	}
	// Query and publication columns are admitted before any hot Rule binds.
	// This gives Link rules their already-sealed typed input axis while the
	// binding is still open; the engine retains only the schema/slot address.
	queriesBound := bindQueries(state, binding, fragments, axes)
	publication, publicationOK := compilation.Publication()
	columnsAdmitted := publicationOK && publication.AdmitColumns(binding)
	if !queriesBound {
		return catalogBinding{}, BindFailure{Stage: BindStageQueries}
	}
	if !columnsAdmitted {
		return catalogBinding{}, BindFailure{Stage: BindStagePublication}
	}
	seal := func() bool { return binding.Seal() }
	rules, failedRule, failedStage := bindRules(state, binding, compilation.catalog.ruleFragments, set, seal)
	if failedStage != RuleBindStageNone {
		if failedStage == RuleBindStageSeal {
			if !queriesBound {
				return catalogBinding{}, BindFailure{Stage: BindStageQueries}
			}
			if !columnsAdmitted {
				return catalogBinding{}, BindFailure{Stage: BindStagePublication}
			}
			return catalogBinding{}, BindFailure{Stage: BindStageSeal}
		}
		return catalogBinding{}, BindFailure{Stage: BindStageRule, Rule: failedRule, RuleStage: failedStage}
	}
	if rules == nil {
		return catalogBinding{}, BindFailure{Stage: BindStageSeal}
	}
	// The sealed query fragments are the canonical query rows. Their typed
	// implementations remain owned by the same sealed binding; no second
	// post-seal receipt table is recovered or retained.
	bound := catalogBinding{catalog: state, binding: binding, rules: rules, allocations: allocations, value: value, queries: fragments}
	if !bound.available() {
		return catalogBinding{}, BindFailure{Stage: BindStageSeal}
	}
	return bound, BindFailure{}
}
