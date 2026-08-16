package analysis

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callactivation "github.com/wippyai/go-lua/analysis/domain/call/activation"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	heapindex "github.com/wippyai/go-lua/analysis/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// programBinding is the one sealed Link-local hot binding for the global
// reusable Program schema. Compile constructs it once; repeated and concurrent
// Plan solves reuse its immutable typed cells while creating independent
// solve-local topology/runtime transactions.
//
// The rule plane is not enumerated here: grammar.Bind drives the sealed rule
// table and this record retains only its neutral hot projection.
type programBinding struct {
	binding *engine.SchemaBinding

	// rules is the sealed rule table's Link-local projection. Every rule
	// admission, attachment, and classification goes through it.
	rules *grammar.RuleBinding

	value *valueowner.HotOwner

	// allocationCatalog is the Link-owned allocation directory admitted with
	// this binding.
	allocationCatalog *allocationcatalog.Catalog

	// runtimeContexts retains only the already-sealed Pack/Heap authorities
	// needed to join an externally-issued runtime context later. It owns no
	// runtime policy or live capability: those remain runtime inputs and are
	// revoked by their authority, not by compilation.
	runtimeContexts runtimeAllocationContextBindingOwner

	valueQuery  *engine.SummaryQueryImplementation[valuedomain.Value, valueSummaryObservation]
	effectQuery *engine.ExactQueryImplementation[effectfactor.Value, effectObservation]
}

// runtimeAllocationContextBindingOwner is Plan-local cold authority for
// creating a Pack/Heap issuer after a runtime supplies its policy identity.
// Keeping this pair in programBinding makes a positive publication proof join
// the exact schemas that produced it; a separate equal-content reseal cannot
// enter through matching scalar IDs.
type runtimeAllocationContextBindingOwner struct {
	pack *packdomain.Schema
	heap heapdomain.Schema
}

func newRuntimeAllocationContextBindingOwner(packSchema *packdomain.Schema, heapSchema heapdomain.Schema) (runtimeAllocationContextBindingOwner, bool) {
	owner := runtimeAllocationContextBindingOwner{pack: packSchema, heap: heapSchema}
	return owner, owner.valid()
}

func (owner runtimeAllocationContextBindingOwner) valid() bool {
	return owner.pack != nil && owner.heap.Valid() && owner.heap.LinkOwner().Matches(owner.pack.LinkOwner())
}

// Begin joins a runtime-owned policy identity to this exact Plan's sealed
// Pack/Heap pair. The returned authority remains short-lived and must be
// closed by the runtime owner; neither this Plan-local owner nor Result keeps
// a live context capability.
func (owner runtimeAllocationContextBindingOwner) Begin(policyID identity.ContentID) (*heapdomain.RuntimeAllocationContextAuthority, packdomain.RuntimeAllocationContextBindingIssuer, bool) {
	if !owner.valid() {
		return nil, packdomain.RuntimeAllocationContextBindingIssuer{}, false
	}
	authority, authorityOK := owner.heap.BeginRuntimeAllocationContexts(policyID)
	if !authorityOK {
		return nil, packdomain.RuntimeAllocationContextBindingIssuer{}, false
	}
	issuer, issuerOK := packdomain.NewRuntimeAllocationContextBindingIssuer(owner.pack, owner.heap, authority)
	if !issuerOK {
		authority.Close()
		return nil, packdomain.RuntimeAllocationContextBindingIssuer{}, false
	}
	return authority, issuer, true
}

// ProgramBindingFailure is the closed Link-local binding boundary. It names
// only the owner transaction that rejected; no Schema slot, callback,
// coordinate, Program proof, or mutable binding state escapes diagnostics.
//
// The per-rule ordinals are derived from the sealed rule table rather than
// restated: a rule failure occupies programBindingFailureRuleBase plus that
// rule's diagnostic ordinal.
type ProgramBindingFailure uint8

const (
	ProgramBindingFailureNone ProgramBindingFailure = iota
	ProgramBindingFailureInput
	ProgramBindingFailureSemantics
	ProgramBindingFailureTypes
	ProgramBindingFailureStatic
	ProgramBindingFailureValueSchema
	ProgramBindingFailureHeapSchema
	ProgramBindingFailurePackSchema
	ProgramBindingFailureCallAlgebra
	ProgramBindingFailureEffectAlgebra
	ProgramBindingFailureHeapIndex
	ProgramBindingFailureTarget
	ProgramBindingFailureTargetCatalog
	ProgramBindingFailureTable
	ProgramBindingFailureReceipt
	ProgramBindingFailureBinding
	ProgramBindingFailurePrincipal
	ProgramBindingFailureAllocationCatalog
	ProgramBindingFailureQueryCatalog
	ProgramBindingFailureSeal
	ProgramBindingFailureAllocationReceipts
	ProgramBindingFailureValueQueryReceipt
	ProgramBindingFailureEffectQueryReceipt
	// programBindingFailureRuleBase is the first ordinal of the derived
	// per-rule tail. Nothing is declared past it.
	programBindingFailureRuleBase
)

var programBindingFailureNames = [...]string{
	"none", "input", "semantics", "types", "static",
	"value-schema", "heap-schema", "pack-schema", "call-algebra", "effect-algebra", "heap-index",
	"target", "target-catalog", "table", "receipt", "binding", "principal",
	"allocation-catalog", "query-catalog", "seal", "allocation-receipts",
	"value-query-receipt", "effect-query-receipt",
}

func (failure ProgramBindingFailure) String() string {
	if failure >= programBindingFailureRuleBase {
		return "rule/" + grammar.DiagnosticRule(failure-programBindingFailureRuleBase).String()
	}
	if int(failure) >= len(programBindingFailureNames) {
		return "invalid"
	}
	return programBindingFailureNames[failure]
}

func programBindingFailureForRule(rule grammar.DiagnosticRule) ProgramBindingFailure {
	return programBindingFailureRuleBase + ProgramBindingFailure(rule)
}

// mountedCapability resolves a mounted rule by its sealed table role.
func (binding *programBinding) mountedCapability(role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
	if binding == nil || binding.rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := binding.rules.Capability(role)
	return capability, ok && capability.Mounted()
}

// linkCapability is the mount-neutral counterpart for Link-owned rules.
func (binding *programBinding) linkCapability(role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
	if binding == nil || binding.rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := binding.rules.Capability(role)
	return capability, ok && capability.Link()
}

// linkOccurrenceIDs enumerates one Link rule's admitted occurrences from its
// own published catalog.
func (binding *programBinding) linkOccurrenceIDs(role programartifact.RuleRole) ([]identity.ContentID, bool) {
	if binding == nil || binding.rules == nil {
		return nil, false
	}
	catalog, ok := binding.rules.LinkCatalog(role)
	if !ok {
		return nil, false
	}
	ids := make([]identity.ContentID, catalog.Count())
	for index := range ids {
		id, idOK := catalog.IDAt(index)
		if !idOK {
			return nil, false
		}
		ids[index] = id
	}
	return ids, true
}

// newProgramBinding binds the complete global schema in one SchemaBinding.
// The factor principals, the allocation catalog, and every rule are admitted
// by the grammar's own transaction; this function supplies the Link
// authorities and the analyzer-owned query step, then adopts the sealed
// result.
func newProgramBinding(
	receipt grammar.CompilationReceipt,
	semantics vocabulary.Bundle,
	valueSchema *valuedomain.Schema,
	callAlgebra *calldomain.Algebra,
	heapSchema heapdomain.Schema,
	heapMounts []heapdomain.ArtifactMount,
	packSchema *packdomain.Schema,
	effectAlgebra *effectfactor.Algebra,
	topology *heapindex.Topology,
	// activationCatalog is the sealed mounted Program artifact/target-batch
	// receipt. It carries no legacy engine assembly, Link, or solve-local plan
	// authority.
	activationCatalog *callactivation.TargetBatchCatalog,
) (*programBinding, ProgramBindingFailure, allocationcatalog.SealFailure) {
	if !semantics.Available() {
		return nil, ProgramBindingFailureSemantics, allocationcatalog.SealFailureNone
	}
	var (
		value           *valueowner.HotOwner
		valueQuerySlot  *engine.QuerySlot[query.ValueSummaryObservation]
		effectQuerySlot *engine.QuerySlot[query.EffectObservation]
	)
	inputs := grammar.LinkInputs{
		ValueSchema:       valueSchema,
		CallAlgebra:       callAlgebra,
		HeapSchema:        heapSchema,
		HeapMounts:        heapMounts,
		PackSchema:        packSchema,
		EffectAlgebra:     effectAlgebra,
		Topology:          topology,
		ActivationCatalog: activationCatalog,
		BindPrincipals: func(valueOwner *valueowner.HotOwner, effectOwner *effectowner.HotOwner, views grammar.QueryViews) bool {
			valueQuery, valueRead, valueViewOK := views.Value()
			effectQuery, _, effectViewOK := views.Effect()
			if !valueViewOK || !effectViewOK {
				return false
			}
			if !valueowner.BindSummaryQuery(valueOwner, valueQuery, valueRead, valueSummaryQueryHotSpec(valueSchema, semantics.ValueCodec)) {
				return false
			}
			if !effectowner.BindExactQuery(effectOwner, effectQuery, effectExactQueryHotSpec(effectAlgebra, semantics.EffectCodec)) {
				return false
			}
			value = valueOwner
			valueQuerySlot, effectQuerySlot = valueQuery, effectQuery
			return true
		},
	}
	bound, failure := grammar.Bind(receipt, inputs)
	if failure.Available() {
		return nil, programBindingFailure(failure), failure.Allocation
	}
	valueQueryReceipt, ok := engine.SummaryQueryImplementationAt[valuedomain.Value, valueSummaryObservation](bound.SchemaBinding(), valueQuerySlot)
	if !ok {
		return nil, ProgramBindingFailureValueQueryReceipt, allocationcatalog.SealFailureNone
	}
	effectQueryReceipt, ok := engine.ExactQueryImplementationAt[effectfactor.Value, effectObservation](bound.SchemaBinding(), effectQuerySlot)
	if !ok {
		return nil, ProgramBindingFailureEffectQueryReceipt, allocationcatalog.SealFailureNone
	}
	runtimeContexts, runtimeContextsOK := newRuntimeAllocationContextBindingOwner(packSchema, heapSchema)
	if !runtimeContextsOK {
		return nil, ProgramBindingFailurePackSchema, allocationcatalog.SealFailureNone
	}
	return &programBinding{
		binding:           bound.SchemaBinding(),
		rules:             bound.Rules(),
		value:             value,
		allocationCatalog: bound.Allocations(),
		runtimeContexts:   runtimeContexts,
		valueQuery:        valueQueryReceipt,
		effectQuery:       effectQueryReceipt,
	}, ProgramBindingFailureNone, allocationcatalog.SealFailureNone
}

// programBindingFailure projects the grammar's closed verdict into the
// analyzer's own boundary. A per-rule phase keeps the exact rule identity.
func programBindingFailure(failure grammar.BindFailure) ProgramBindingFailure {
	switch failure.Stage {
	case grammar.BindStageInput:
		return ProgramBindingFailureInput
	case grammar.BindStageTable:
		return ProgramBindingFailureTable
	case grammar.BindStageReceipt:
		return ProgramBindingFailureReceipt
	case grammar.BindStageBinding:
		return ProgramBindingFailureBinding
	case grammar.BindStagePrincipal:
		return ProgramBindingFailurePrincipal
	case grammar.BindStageAllocations:
		return ProgramBindingFailureAllocationCatalog
	case grammar.BindStageRule:
		return programBindingFailureForRule(failure.Rule)
	case grammar.BindStageQueries:
		return ProgramBindingFailureQueryCatalog
	case grammar.BindStageSeal:
		return ProgramBindingFailureSeal
	case grammar.BindStageAllocationReceipts:
		return ProgramBindingFailureAllocationReceipts
	default:
		return ProgramBindingFailureNone
	}
}
