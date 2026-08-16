package analysis

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callactivation "github.com/wippyai/go-lua/analysis/domain/call/activation"
	calldispatch "github.com/wippyai/go-lua/analysis/domain/call/dispatch"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	callsite "github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	heapclosed "github.com/wippyai/go-lua/analysis/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/analysis/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/analysis/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/analysis/domain/heap/bootstrap"
	heapindex "github.com/wippyai/go-lua/analysis/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/analysis/domain/pack/source"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueallocation "github.com/wippyai/go-lua/analysis/domain/value/allocation"
	valuearithmetic "github.com/wippyai/go-lua/analysis/domain/value/arithmetic"
	valuebootstrap "github.com/wippyai/go-lua/analysis/domain/value/bootstrap"
	valueequality "github.com/wippyai/go-lua/analysis/domain/value/equality"
	valueorder "github.com/wippyai/go-lua/analysis/domain/value/order"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuerefinement "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	valuesource "github.com/wippyai/go-lua/analysis/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/analysis/domain/value/transfer"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/analysis/internal/semanticvocabulary"
	"github.com/wippyai/go-lua/program/keyspace"
)

// programBinding is the one sealed Link-local hot binding for the global
// reusable Program schema. Compile constructs it once; repeated and concurrent
// Plan solves reuse its immutable typed cells while creating independent
// solve-local topology/runtime transactions.
type programBinding struct {
	binding *engine.SchemaBinding
	// vocabulary is the one canonical semantic Bundle used to resolve rule
	// capabilities after the binding seals.  Rule capabilities are not cached
	// here: callers must resolve them through the binding's exact directory.
	vocabulary semanticvocabulary.Bundle

	value *valueowner.HotOwner

	valueSource     *valuesource.HotRule
	packSource      *packsource.HotRule
	heapIngress     *heapingress.HotRule
	valueAllocation *valueallocation.HotRule
	heapEmpty       *heapempty.HotRule
	heapClosed      *heapclosed.HotRule
	rawGet          *heapindex.RawGetHotRule
	rawSet          *heapindex.RawSetHotRule
	callDispatch    *calldispatch.HotRule
	effectSelected  *callsite.HotRule
	effectOpaque    *callsite.HotRule
	effectBody      *callsite.BodyHotRule
	callActivation  *callactivation.HotRule
	valueBootstrap  *valuebootstrap.HotRule
	heapBootstrap   *heapbootstrap.HotRule
	valueTransfer   *valuetransfer.HotRule
	valueArithmetic *valuearithmetic.HotRule
	valueEquality   *valueequality.HotRule
	valueOrder      *valueorder.HotRule
	valueRefinement *valuerefinement.HotRule

	// Sealed Link-local directories retained for later mounted substitution.
	// Pack and Effect keep their occurrence issuers inside their hot rules;
	// these catalogs have independent typed query surfaces.
	allocationCatalog     *allocationcatalog.Catalog
	valueBootstrapCatalog *valuebootstrap.Catalog
	heapBootstrapCatalog  *heapbootstrap.Catalog

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
func (owner runtimeAllocationContextBindingOwner) Begin(policyID keyspace.ContentID) (*heapdomain.RuntimeAllocationContextAuthority, packdomain.RuntimeAllocationContextBindingIssuer, bool) {
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
type ProgramBindingFailure uint8

const (
	ProgramBindingFailureNone ProgramBindingFailure = iota
	ProgramBindingFailureInput
	ProgramBindingFailureSemantics
	ProgramBindingFailureTypes
	ProgramBindingFailureStatic
	ProgramBindingFailureHeapSchema
	ProgramBindingFailureValueSchema
	ProgramBindingFailurePackSchema
	ProgramBindingFailureCallAlgebra
	ProgramBindingFailureTarget
	ProgramBindingFailureEffectAlgebra
	ProgramBindingFailureHeapIndex
	ProgramBindingFailureTargetCatalog
	ProgramBindingFailureCatalog
	ProgramBindingFailureSchema
	ProgramBindingFailureValueOwner
	ProgramBindingFailureCallOwner
	ProgramBindingFailureHeapOwner
	ProgramBindingFailurePackOwner
	ProgramBindingFailureEffectOwner
	ProgramBindingFailureAllocationCatalog
	ProgramBindingFailureValueSource
	ProgramBindingFailurePackSource
	ProgramBindingFailureHeapIngress
	ProgramBindingFailureValueAllocation
	ProgramBindingFailureHeapEmpty
	ProgramBindingFailureHeapClosed
	ProgramBindingFailureRawGet
	ProgramBindingFailureRawSet
	ProgramBindingFailureCallDispatch
	ProgramBindingFailureEffectSelected
	ProgramBindingFailureEffectOpaque
	ProgramBindingFailureEffectBody
	ProgramBindingFailureCallActivation
	ProgramBindingFailureActivationTransport
	ProgramBindingFailureValueBootstrap
	ProgramBindingFailureHeapBootstrap
	ProgramBindingFailureValueTransfer
	ProgramBindingFailureValueArithmetic
	ProgramBindingFailureValueEquality
	ProgramBindingFailureValueOrder
	ProgramBindingFailureValueRefinement
	ProgramBindingFailureRoleDirectory
	ProgramBindingFailureQueryCatalog
	ProgramBindingFailureValueQuery
	ProgramBindingFailureEffectQuery
	ProgramBindingFailureSeal
	ProgramBindingFailureCallOccurrences
	ProgramBindingFailurePackOccurrences
	ProgramBindingFailureEffectOccurrences
	ProgramBindingFailureHeapIngressCatalog
	ProgramBindingFailureBootstrapCatalog
	ProgramBindingFailureValueQueryReceipt
	ProgramBindingFailureEffectQueryReceipt
)

func (failure ProgramBindingFailure) String() string {
	names := [...]string{
		"none", "input", "semantics", "types", "static", "heap-schema", "value-schema", "pack-schema", "call-algebra", "target",
		"effect-algebra", "heap-index", "target-catalog", "catalog", "schema", "value-owner", "call-owner", "heap-owner", "pack-owner", "effect-owner",
		"allocation-catalog", "value-source", "pack-source", "heap-ingress", "value-allocation", "heap-empty", "heap-closed",
		"raw-get", "raw-set", "call-dispatch", "effect-selected", "effect-opaque", "effect-body", "call-activation",
		"activation-transport", "value-bootstrap", "heap-bootstrap", "value-transfer", "value-binary-arithmetic", "value-binary-equality", "value-binary-order", "value-presence-refinement", "role-directory", "query-catalog",
		"value-query", "effect-query", "seal", "call-occurrences", "pack-occurrences", "effect-occurrences",
		"heap-ingress-catalog", "bootstrap-catalog", "value-query-receipt", "effect-query-receipt",
	}
	if int(failure) < 0 || int(failure) >= len(names) {
		return "invalid"
	}
	return names[failure]
}

// registerMountedRuleSlot performs the short-lived pre-seal owner handoff.
// The capability is deliberately not retained: once SchemaBinding seals,
// callers resolve the exact capability from its semantic directory.
func registerMountedRuleSlot[V, O any](binding *engine.SchemaBinding, slot *engine.RuleSlot[V, O]) bool {
	capability, ok := engine.IssueMountedRuleCapability(binding, slot)
	return ok && engine.RegisterRuleSlot(binding, slot, capability)
}

func registerActivationRuleSlot(binding *engine.SchemaBinding, slot *engine.SchemaActivationRuleSlot) bool {
	capability, ok := engine.IssueActivationRuleCapability(binding, slot)
	return ok && engine.RegisterActivationRuleSlot(binding, slot, capability)
}

func registerLinkRuleSlot[V, O any](binding *engine.SchemaBinding, slot *engine.RuleSlot[V, O]) (engine.RuleSlotCapability, bool) {
	capability, ok := engine.IssueLinkRuleCapability(binding, slot)
	return capability, ok && engine.RegisterRuleSlot(binding, slot, capability)
}

// mountedCapability resolves a mounted rule by the canonical semantic key.
// SchemaBinding authenticates the key against its exact rule ordinal and
// authority; no Link-local capability cache or role registry is needed.
func (binding *programBinding) mountedCapability(semantic engine.SemanticKey) (engine.RuleSlotCapability, bool) {
	if binding == nil || binding.binding == nil || !semantic.Available() {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := engine.BindingRuleSlot(binding.binding, semantic)
	return capability, ok && capability.Mounted()
}

// linkCapability is the mount-neutral counterpart for bootstrap rules.
func (binding *programBinding) linkCapability(semantic engine.SemanticKey) (engine.RuleSlotCapability, bool) {
	if binding == nil || binding.binding == nil || !semantic.Available() {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := engine.BindingRuleSlot(binding.binding, semantic)
	return capability, ok && capability.Link()
}

// newProgramBinding binds the complete global schema in one SchemaBinding.
// Every domain receives its exact cold fragment and Link-owned algebra; no
// legacy Rule, Composition, or raw slot is accepted by this transaction.
func newProgramBinding(
	receipt programschema.CompilationReceipt,
	semantics semanticvocabulary.Bundle,
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
	if !receipt.Available() || valueSchema == nil || callAlgebra == nil || !callAlgebra.Valid() || !heapSchema.Valid() || len(heapMounts) == 0 || packSchema == nil || effectAlgebra == nil || !effectAlgebra.Valid() || topology == nil || activationCatalog == nil {
		return nil, ProgramBindingFailureInput, allocationcatalog.SealFailureNone
	}
	if !semantics.Available() {
		return nil, ProgramBindingFailureSemantics, allocationcatalog.SealFailureNone
	}
	catalog, ok := receipt.BindingCatalog()
	if !ok || catalog.Value() == nil || catalog.Call() == nil || catalog.Heap() == nil || catalog.Pack() == nil || catalog.Effect() == nil {
		return nil, ProgramBindingFailureCatalog, allocationcatalog.SealFailureNone
	}
	binding := engine.NewSchemaBinding(receipt.Schema())
	if binding == nil {
		return nil, ProgramBindingFailureSchema, allocationcatalog.SealFailureNone
	}
	value, ok := valueowner.BindHot(binding, catalog.Value(), valueSchema)
	if !ok {
		return nil, ProgramBindingFailureValueOwner, allocationcatalog.SealFailureNone
	}
	call, ok := callowner.BindHot(binding, catalog.Call(), callAlgebra)
	if !ok {
		return nil, ProgramBindingFailureCallOwner, allocationcatalog.SealFailureNone
	}
	heap, ok := heapowner.BindHot(binding, catalog.Heap(), heapSchema)
	if !ok {
		return nil, ProgramBindingFailureHeapOwner, allocationcatalog.SealFailureNone
	}
	pack, ok := packowner.BindHot(binding, catalog.Pack(), packSchema)
	if !ok {
		return nil, ProgramBindingFailurePackOwner, allocationcatalog.SealFailureNone
	}
	effect, ok := effectowner.BindHot(binding, catalog.Effect(), effectAlgebra)
	if !ok {
		return nil, ProgramBindingFailureEffectOwner, allocationcatalog.SealFailureNone
	}
	allocations, allocationFailure := allocationcatalog.BeginWithFailure(heapSchema, valueSchema, value, heapMounts)
	if allocationFailure != allocationcatalog.SealFailureNone {
		return nil, ProgramBindingFailureAllocationCatalog, allocationFailure
	}

	valueSource, ok := valuesource.BindHot(catalog.ValueSource(), value)
	if !ok {
		return nil, ProgramBindingFailureValueSource, allocationcatalog.SealFailureNone
	}
	packSource, ok := packsource.BindHot(catalog.PackSource(), pack, packSchema)
	if !ok {
		return nil, ProgramBindingFailurePackSource, allocationcatalog.SealFailureNone
	}
	heapIngress, ok := heapingress.BindHot(catalog.HeapIngress(), heap)
	if !ok {
		return nil, ProgramBindingFailureHeapIngress, allocationcatalog.SealFailureNone
	}
	valueAllocation, ok := valueallocation.BindHot(catalog.ValueAllocation(), value, heapSchema, allocations)
	if !ok {
		return nil, ProgramBindingFailureValueAllocation, allocationcatalog.SealFailureNone
	}
	heapEmpty, ok := heapempty.BindHot(catalog.HeapEmpty(), heap, allocations)
	if !ok {
		return nil, ProgramBindingFailureHeapEmpty, allocationcatalog.SealFailureNone
	}
	heapClosed, ok := heapclosed.BindHot(binding, catalog.HeapClosed(), heap, value, allocations)
	if !ok {
		return nil, ProgramBindingFailureHeapClosed, allocationcatalog.SealFailureNone
	}
	rawGet, ok := heapindex.BindRawGetHot(binding, catalog.RawGet(), topology, value, call, heap, pack)
	if !ok {
		return nil, ProgramBindingFailureRawGet, allocationcatalog.SealFailureNone
	}
	rawSet, ok := heapindex.BindRawSetHot(binding, catalog.RawSet(), topology, value, heap, pack)
	if !ok {
		return nil, ProgramBindingFailureRawSet, allocationcatalog.SealFailureNone
	}
	callDispatch, ok := calldispatch.BindHot(binding, catalog.CallDispatch(), value, call, heapSchema, packSchema)
	if !ok {
		return nil, ProgramBindingFailureCallDispatch, allocationcatalog.SealFailureNone
	}
	effectSelected, ok := callsite.BindSelectedHot(binding, catalog.EffectSelected(), call, effect)
	if !ok {
		return nil, ProgramBindingFailureEffectSelected, allocationcatalog.SealFailureNone
	}
	effectOpaque, ok := callsite.BindOpaqueHot(binding, catalog.EffectOpaque(), call, effect)
	if !ok {
		return nil, ProgramBindingFailureEffectOpaque, allocationcatalog.SealFailureNone
	}
	effectBody, ok := callsite.BindBodyHot(binding, catalog.EffectBody(), call, effect)
	if !ok {
		return nil, ProgramBindingFailureEffectBody, allocationcatalog.SealFailureNone
	}
	callActivation, ok := callactivation.BindHot(catalog.CallActivation(), call, activationCatalog)
	if !ok {
		return nil, ProgramBindingFailureCallActivation, allocationcatalog.SealFailureNone
	}
	if !callactivation.BindMountedTransport(callActivation, value.FactorRef(), call.FactorRef(), heap.FactorRef(), pack.FactorRef(), effect.FactorRef()) {
		return nil, ProgramBindingFailureActivationTransport, allocationcatalog.SealFailureNone
	}
	valueBootstrap, ok := valuebootstrap.BindHot(catalog.ValueBootstrap(), value)
	if !ok {
		return nil, ProgramBindingFailureValueBootstrap, allocationcatalog.SealFailureNone
	}
	heapBootstrap, ok := heapbootstrap.BindHot(catalog.HeapBootstrap(), heap)
	if !ok {
		return nil, ProgramBindingFailureHeapBootstrap, allocationcatalog.SealFailureNone
	}
	valueTransfer, ok := valuetransfer.BindHot(catalog.ValueTransfer(), value)
	if !ok {
		return nil, ProgramBindingFailureValueTransfer, allocationcatalog.SealFailureNone
	}
	valueArithmetic, ok := valuearithmetic.BindHot(catalog.ValueArithmetic(), value)
	if !ok {
		return nil, ProgramBindingFailureValueArithmetic, allocationcatalog.SealFailureNone
	}
	valueEquality, ok := valueequality.BindHot(catalog.ValueEquality(), value)
	if !ok {
		return nil, ProgramBindingFailureValueEquality, allocationcatalog.SealFailureNone
	}
	valueOrder, ok := valueorder.BindHot(catalog.ValueOrder(), value)
	if !ok {
		return nil, ProgramBindingFailureValueOrder, allocationcatalog.SealFailureNone
	}
	valueRefinement, ok := valuerefinement.BindHot(catalog.ValueRefinement(), value)
	if !ok {
		return nil, ProgramBindingFailureValueRefinement, allocationcatalog.SealFailureNone
	}
	// Register one domain-owned slot at a time so the first rejected slot keeps
	// its exact closed owner classification. The issued capability dies with
	// this call; the sealed binding is the sole later lookup authority.
	if !registerMountedRuleSlot(binding, catalog.ValueSource().RuleSlot()) {
		return nil, ProgramBindingFailureValueSource, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.PackSource().RuleSlot()) {
		return nil, ProgramBindingFailurePackSource, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.HeapIngress().RuleSlot()) {
		return nil, ProgramBindingFailureHeapIngress, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.ValueAllocation().RuleSlot()) {
		return nil, ProgramBindingFailureValueAllocation, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.HeapEmpty().RuleSlot()) {
		return nil, ProgramBindingFailureHeapEmpty, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.HeapClosed().RuleSlot()) {
		return nil, ProgramBindingFailureHeapClosed, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.RawGet().RuleSlot()) {
		return nil, ProgramBindingFailureRawGet, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.RawSet().RuleSlot()) {
		return nil, ProgramBindingFailureRawSet, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.CallDispatch().RuleSlot()) {
		return nil, ProgramBindingFailureCallDispatch, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.EffectSelected().RuleSlot()) {
		return nil, ProgramBindingFailureEffectSelected, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.EffectOpaque().RuleSlot()) {
		return nil, ProgramBindingFailureEffectOpaque, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.EffectBody().RuleSlot()) {
		return nil, ProgramBindingFailureEffectBody, allocationcatalog.SealFailureNone
	}
	if !registerActivationRuleSlot(binding, catalog.CallActivation().ActivationSlot()) {
		return nil, ProgramBindingFailureCallActivation, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.ValueTransfer().RuleSlot()) {
		return nil, ProgramBindingFailureValueTransfer, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.ValueArithmetic().RuleSlot()) {
		return nil, ProgramBindingFailureValueArithmetic, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.ValueEquality().RuleSlot()) {
		return nil, ProgramBindingFailureValueEquality, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.ValueOrder().RuleSlot()) {
		return nil, ProgramBindingFailureValueOrder, allocationcatalog.SealFailureNone
	}
	if !registerMountedRuleSlot(binding, catalog.ValueRefinement().RuleSlot()) {
		return nil, ProgramBindingFailureValueRefinement, allocationcatalog.SealFailureNone
	}
	valueBootstrapCapability, valueBootstrapCapabilityOK := registerLinkRuleSlot(binding, catalog.ValueBootstrap().RuleSlot())
	if !valueBootstrapCapabilityOK {
		return nil, ProgramBindingFailureValueBootstrap, allocationcatalog.SealFailureNone
	}
	heapBootstrapCapability, heapBootstrapCapabilityOK := registerLinkRuleSlot(binding, catalog.HeapBootstrap().RuleSlot())
	if !heapBootstrapCapabilityOK || !engine.RegisterLinkBootstrapTransportPair(binding, valueBootstrapCapability, heapBootstrapCapability) {
		return nil, ProgramBindingFailureHeapBootstrap, allocationcatalog.SealFailureNone
	}

	queries, ok := receipt.Queries()
	if !ok {
		return nil, ProgramBindingFailureQueryCatalog, allocationcatalog.SealFailureNone
	}
	valueQuery, valueRead, ok := queries.Value()
	if !ok || !valueowner.BindSummaryQuery(value, valueQuery, valueRead, valueSummaryQueryHotSpec(valueSchema, semantics.ValueCodec)) {
		return nil, ProgramBindingFailureValueQuery, allocationcatalog.SealFailureNone
	}
	effectQuery, _, ok := queries.Effect()
	if !ok || !effectowner.BindExactQuery(effect, effectQuery, effectExactQueryHotSpec(effectAlgebra, semantics.EffectCodec)) {
		return nil, ProgramBindingFailureEffectQuery, allocationcatalog.SealFailureNone
	}
	if !binding.Seal() {
		return nil, ProgramBindingFailureSeal, allocationcatalog.SealFailureNone
	}
	if allocationFailure = allocations.SealSummaryReceiptsWithFailure(); allocationFailure != allocationcatalog.SealFailureNone {
		return nil, ProgramBindingFailureAllocationCatalog, allocationFailure
	}
	// Mounted occurrence issuers are sealed only after the shared binding is
	// terminal.  Every failure below therefore closes this transaction before
	// exposing a partially attached Link-local binding.
	if !callDispatch.SealOccurrenceReceipts() {
		return nil, ProgramBindingFailureCallOccurrences, allocationcatalog.SealFailureNone
	}
	if !packSource.SealOccurrenceReceipts() {
		return nil, ProgramBindingFailurePackOccurrences, allocationcatalog.SealFailureNone
	}
	if !effectSelected.SealOccurrenceReceipts() || !effectOpaque.SealOccurrenceReceipts() || !effectBody.SealOccurrenceReceipts() || !callActivation.SealOccurrenceReceipts() {
		return nil, ProgramBindingFailureEffectOccurrences, allocationcatalog.SealFailureNone
	}
	if !heapIngress.AttachCatalog(allocations) {
		return nil, ProgramBindingFailureHeapIngressCatalog, allocationcatalog.SealFailureNone
	}
	valueBootstrapCatalog := valueBootstrap.Catalog()
	heapBootstrapCatalog := heapBootstrap.Catalog()
	if valueBootstrapCatalog == nil || !valueBootstrapCatalog.FencedTo(valueSchema) ||
		heapBootstrapCatalog == nil || !heapBootstrapCatalog.FencedTo(heapSchema) {
		return nil, ProgramBindingFailureBootstrapCatalog, allocationcatalog.SealFailureNone
	}
	valueQueryReceipt, ok := engine.SummaryQueryImplementationAt[valuedomain.Value, valueSummaryObservation](binding, valueQuery)
	if !ok {
		return nil, ProgramBindingFailureValueQueryReceipt, allocationcatalog.SealFailureNone
	}
	effectQueryReceipt, ok := engine.ExactQueryImplementationAt[effectfactor.Value, effectObservation](binding, effectQuery)
	if !ok {
		return nil, ProgramBindingFailureEffectQueryReceipt, allocationcatalog.SealFailureNone
	}
	runtimeContexts, runtimeContextsOK := newRuntimeAllocationContextBindingOwner(packSchema, heapSchema)
	if !runtimeContextsOK {
		return nil, ProgramBindingFailurePackSchema, allocationcatalog.SealFailureNone
	}
	return &programBinding{
		binding:     binding,
		vocabulary:  semantics,
		value:       value,
		valueSource: valueSource, packSource: packSource, heapIngress: heapIngress,
		valueAllocation: valueAllocation, heapEmpty: heapEmpty, heapClosed: heapClosed,
		rawGet: rawGet, rawSet: rawSet, callDispatch: callDispatch,
		effectSelected: effectSelected, effectOpaque: effectOpaque, effectBody: effectBody,
		callActivation: callActivation, valueBootstrap: valueBootstrap, heapBootstrap: heapBootstrap,
		valueTransfer: valueTransfer, valueArithmetic: valueArithmetic, valueEquality: valueEquality, valueOrder: valueOrder, valueRefinement: valueRefinement, allocationCatalog: allocations,
		valueBootstrapCatalog: valueBootstrapCatalog, heapBootstrapCatalog: heapBootstrapCatalog,
		runtimeContexts: runtimeContexts,
		valueQuery:      valueQueryReceipt, effectQuery: effectQueryReceipt,
	}, ProgramBindingFailureNone, allocationcatalog.SealFailureNone
}
