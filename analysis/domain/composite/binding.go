package composite

import (
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
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
	BindStageSeal
	BindStageAllocations
	BindStageValueQueryReceipt
	BindStageEffectQueryReceipt
	BindStageRuntimeContexts
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
	case BindStageSeal:
		return "seal"
	case BindStageAllocations:
		return "allocations"
	case BindStageValueQueryReceipt:
		return "value-query-receipt"
	case BindStageEffectQueryReceipt:
		return "effect-query-receipt"
	case BindStageRuntimeContexts:
		return "runtime-contexts"
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
// sealed engine binding, the rule table's hot projection, and the allocation
// catalog the Link owns. It is the neutral half of the one published binding
// record, ProgramBinding, which adds the principals' query implementations.
type catalogBinding struct {
	binding     *engine.SchemaBinding
	rules       *RuleBinding
	allocations *allocationcatalog.Catalog
}

func (bound catalogBinding) available() bool {
	return bound.binding != nil && bound.binding.Sealed() && bound.rules != nil && bound.allocations != nil
}

// bind is the one hot binding transaction for the global reusable catalog. It
// binds the factor principals, opens the Link allocation catalog, drives the
// whole rule table, hands the principals to the caller's query step for the
// duration of that call, seals, and finalizes.
//
// Every rule-plane sequence is a pass over the sealed table. The caller
// supplies Link authorities and its own query binder and receives back only
// the neutral execution surface.
func bind(compilation Compilation, inputs LinkInputs, bindPrincipals func(value *valueowner.HotOwner, effect *effectowner.HotOwner, views QueryViews) bool) (catalogBinding, BindFailure) {
	sealRegistry()
	if registry.sealed == nil {
		return catalogBinding{}, BindFailure{Stage: BindStageTable}
	}
	if !inputs.available() || bindPrincipals == nil {
		return catalogBinding{}, BindFailure{Stage: BindStageInput}
	}
	if !compilation.Available() || !compilation.catalog.axisFragments.available() {
		return catalogBinding{}, BindFailure{Stage: BindStageCompilation}
	}
	binding := engine.NewSchemaBinding(compilation.Schema())
	if binding == nil {
		return catalogBinding{}, BindFailure{Stage: BindStageBinding}
	}
	// The axis pass is first: every factor principal is bound by its own
	// declared axis, in the sealed table's order, before any rule or catalog
	// consumes one.
	axes, failedAxis, axesOK := bindAxes(binding, compilation.catalog.axisFragments, inputs)
	if !axesOK {
		return catalogBinding{}, BindFailure{Stage: BindStagePrincipal, Axis: failedAxis}
	}
	value, valueOK := axisPayload[*valueowner.HotOwner](axes, programartifact.RuleOutputValue)
	effect, effectOK := axisPayload[*effectowner.HotOwner](axes, programartifact.RuleOutputEffect)
	if !valueOK || !effectOK {
		return catalogBinding{}, BindFailure{Stage: BindStagePrincipal}
	}
	// The mount set the allocation catalog joins is the heap schema's own: it
	// sealed those mounts, so the list is read back from it rather than carried
	// beside it as a second copy.
	allocations, allocationFailure := allocationcatalog.BeginWithFailure(inputs.HeapSchema, inputs.ValueSchema, value, inputs.HeapSchema.ArtifactMounts())
	if allocationFailure != allocationcatalog.SealFailureNone {
		return catalogBinding{}, BindFailure{Stage: BindStageAllocationCatalog, Allocation: allocationFailure}
	}
	set, setOK := axes.hotPrincipals(inputs, allocations)
	if !setOK {
		return catalogBinding{}, BindFailure{Stage: BindStagePrincipal}
	}
	views, viewsOK := compilation.Queries()
	if !viewsOK {
		return catalogBinding{}, BindFailure{Stage: BindStageQueries}
	}
	// The query surface is still the caller's. Its principals are handed over
	// for exactly the duration of this call rather than published as accessors,
	// at the one lawful position: after every rule slot is registered and
	// paired, and before the shared binding becomes terminal.
	queriesBound := false
	seal := func() bool {
		queriesBound = bindPrincipals(value, effect, views)
		return queriesBound && binding.Seal()
	}
	rules, failedRule, failedStage := bindRules(binding, compilation.catalog.ruleFragments, set, seal)
	if failedStage != RuleBindStageNone {
		if failedStage == RuleBindStageSeal {
			if !queriesBound {
				return catalogBinding{}, BindFailure{Stage: BindStageQueries}
			}
			return catalogBinding{}, BindFailure{Stage: BindStageSeal}
		}
		return catalogBinding{}, BindFailure{Stage: BindStageRule, Rule: failedRule, RuleStage: failedStage}
	}
	if rules == nil {
		return catalogBinding{}, BindFailure{Stage: BindStageSeal}
	}
	if allocationFailure = allocations.SealSummaryReceiptsWithFailure(); allocationFailure != allocationcatalog.SealFailureNone {
		return catalogBinding{}, BindFailure{Stage: BindStageAllocations, Allocation: allocationFailure}
	}
	bound := catalogBinding{binding: binding, rules: rules, allocations: allocations}
	if !bound.available() {
		return catalogBinding{}, BindFailure{Stage: BindStageSeal}
	}
	return bound, BindFailure{}
}
