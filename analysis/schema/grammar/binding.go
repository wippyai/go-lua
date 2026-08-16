package grammar

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
	BindStageReceipt
	BindStageBinding
	BindStagePrincipal
	BindStageAllocations
	BindStageRule
	BindStageQueries
	BindStageSeal
	BindStageAllocationReceipts
)

func (stage BindStage) String() string {
	switch stage {
	case BindStageInput:
		return "input"
	case BindStageTable:
		return "table"
	case BindStageReceipt:
		return "receipt"
	case BindStageBinding:
		return "binding"
	case BindStagePrincipal:
		return "principal"
	case BindStageAllocations:
		return "allocations"
	case BindStageRule:
		return "rule"
	case BindStageQueries:
		return "queries"
	case BindStageSeal:
		return "seal"
	case BindStageAllocationReceipts:
		return "allocation-receipts"
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

// Binding is the sealed Link-local binding of the whole grammar. It publishes
// the neutral execution surface only: the sealed engine binding, the rule
// table's hot projection, and the allocation catalog the Link owns. No factor
// principal or domain algebra is reachable through it.
type Binding struct {
	binding     *engine.SchemaBinding
	rules       *RuleBinding
	allocations *allocationcatalog.Catalog
}

func (bound *Binding) Available() bool {
	return bound != nil && bound.binding != nil && bound.binding.Sealed() && bound.rules != nil && bound.allocations != nil
}

// SchemaBinding is the one sealed engine binding this transaction produced.
func (bound *Binding) SchemaBinding() *engine.SchemaBinding {
	if bound == nil {
		return nil
	}
	return bound.binding
}

// Rules is the hot projection of the sealed rule table.
func (bound *Binding) Rules() *RuleBinding {
	if bound == nil {
		return nil
	}
	return bound.rules
}

// Allocations is the Link-owned allocation catalog admitted with this binding.
func (bound *Binding) Allocations() *allocationcatalog.Catalog {
	if bound == nil {
		return nil
	}
	return bound.allocations
}

// Bind is the one hot binding transaction for the global reusable grammar. It
// binds the factor principals, opens the Link allocation catalog, drives the
// whole rule table, hands the principals to the caller's query step for the
// duration of that call, seals, and finalizes.
//
// Every rule-plane sequence is a pass over the sealed table. The caller
// supplies Link authorities and its own query binder and receives back only
// the neutral execution surface.
func Bind(receipt CompilationReceipt, inputs LinkInputs) (*Binding, BindFailure) {
	sealRegistry()
	if registry.sealed == nil {
		return nil, BindFailure{Stage: BindStageTable}
	}
	if !inputs.available() {
		return nil, BindFailure{Stage: BindStageInput}
	}
	if !receipt.Available() || receipt.catalog == nil || !receipt.catalog.axisFragments.available() {
		return nil, BindFailure{Stage: BindStageReceipt}
	}
	binding := engine.NewSchemaBinding(receipt.Schema())
	if binding == nil {
		return nil, BindFailure{Stage: BindStageBinding}
	}
	// The axis pass is first: every factor principal is bound by its own
	// declared axis, in the sealed table's order, before any rule or catalog
	// consumes one.
	axes, failedAxis, axesOK := bindAxes(binding, receipt.catalog.axisFragments, inputs)
	if !axesOK {
		return nil, BindFailure{Stage: BindStagePrincipal, Axis: failedAxis}
	}
	value, valueOK := axisPayload[*valueowner.HotOwner](axes, programartifact.RuleOutputValue)
	effect, effectOK := axisPayload[*effectowner.HotOwner](axes, programartifact.RuleOutputEffect)
	if !valueOK || !effectOK {
		return nil, BindFailure{Stage: BindStagePrincipal}
	}
	allocations, allocationFailure := allocationcatalog.BeginWithFailure(inputs.HeapSchema, inputs.ValueSchema, value, inputs.HeapMounts)
	if allocationFailure != allocationcatalog.SealFailureNone {
		return nil, BindFailure{Stage: BindStageAllocations, Allocation: allocationFailure}
	}
	set, setOK := axes.hotPrincipals(inputs, allocations)
	if !setOK {
		return nil, BindFailure{Stage: BindStagePrincipal}
	}
	views, viewsOK := receipt.Queries()
	if !viewsOK {
		return nil, BindFailure{Stage: BindStageQueries}
	}
	// The query surface is still the caller's. Its principals are handed over
	// for exactly the duration of this call rather than published as accessors,
	// at the one lawful position: after every rule slot is registered and
	// paired, and before the shared binding becomes terminal.
	queriesBound := false
	seal := func() bool {
		queriesBound = inputs.BindPrincipals(value, effect, views)
		return queriesBound && binding.Seal()
	}
	rules, failedRule, failedStage := bindRules(binding, receipt.catalog.ruleFragments, set, seal)
	if failedStage != RuleBindStageNone {
		if failedStage == RuleBindStageSeal {
			if !queriesBound {
				return nil, BindFailure{Stage: BindStageQueries}
			}
			return nil, BindFailure{Stage: BindStageSeal}
		}
		return nil, BindFailure{Stage: BindStageRule, Rule: failedRule, RuleStage: failedStage}
	}
	if rules == nil {
		return nil, BindFailure{Stage: BindStageSeal}
	}
	if allocationFailure = allocations.SealSummaryReceiptsWithFailure(); allocationFailure != allocationcatalog.SealFailureNone {
		return nil, BindFailure{Stage: BindStageAllocationReceipts, Allocation: allocationFailure}
	}
	bound := &Binding{binding: binding, rules: rules, allocations: allocations}
	if !bound.Available() {
		return nil, BindFailure{Stage: BindStageSeal}
	}
	return bound, BindFailure{}
}
