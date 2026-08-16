// Package programschema owns the one process-global cold analyzer grammar.
// Its receipt is the only authority accepted by the Program transformer;
// callers cannot manufacture an equivalent authority from a digest.
package programschema

import (
	"sync"

	callactivation "github.com/wippyai/go-lua/analysis/domain/call/activation"
	calldispatch "github.com/wippyai/go-lua/analysis/domain/call/dispatch"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	callsite "github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapclosed "github.com/wippyai/go-lua/analysis/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/analysis/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/analysis/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/analysis/domain/heap/bootstrap"
	heapindex "github.com/wippyai/go-lua/analysis/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
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
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programquery"
	"github.com/wippyai/go-lua/analysis/internal/semanticvocabulary"
	"github.com/wippyai/go-lua/program/keyspace"
)

const ReceiptVersion uint64 = programartifact.GrammarABIVersion

type catalog struct {
	schema          *engine.Schema
	value           *valueowner.SchemaFragment
	call            *callowner.SchemaFragment
	heap            *heapowner.SchemaFragment
	pack            *packowner.SchemaFragment
	effect          *effectowner.SchemaFragment
	valueSource     *valuesource.SchemaFragment
	packSource      *packsource.SchemaFragment
	heapIngress     *heapingress.SchemaFragment
	valueAllocation *valueallocation.SchemaFragment
	heapEmpty       *heapempty.SchemaFragment
	heapClosed      *heapclosed.SchemaFragment
	rawGet          *heapindex.RawGetSchemaFragment
	rawSet          *heapindex.RawSetSchemaFragment
	callDispatch    *calldispatch.SchemaFragment
	effectSelected  *callsite.SelectedSchemaFragment
	effectOpaque    *callsite.OpaqueSchemaFragment
	effectBody      *callsite.BodySchemaFragment
	callActivation  *callactivation.SchemaFragment
	valueBootstrap  *valuebootstrap.SchemaFragment
	heapBootstrap   *heapbootstrap.SchemaFragment
	valueTransfer   *valuetransfer.SchemaFragment
	valueArithmetic *valuearithmetic.SchemaFragment
	valueEquality   *valueequality.SchemaFragment
	valueOrder      *valueorder.SchemaFragment
	valueRefinement *valuerefinement.SchemaFragment
	valueQuery      *engine.QuerySlot[programquery.ValueSummaryObservation]
	valueRead       engine.SchemaReadForm[valuedomain.Value]
	effectQuery     *engine.QuerySlot[programquery.EffectObservation]
	effectRead      engine.SchemaReadForm[effectfactor.Value]
}

// CompilationReceipt is an opaque proof of the exact sealed schema owner.
// The digest is a view of the proof, never a constructor input.
type CompilationReceipt struct {
	catalog *catalog
	digest  keyspace.ContentID
	version uint64
}

// BindingCatalog is the opaque typed projection consumed by the production
// hot binder.  It exposes cold fragment identities, never their underlying
// Schema slots or the mutable catalog itself.
type BindingCatalog struct{ catalog *catalog }

// BindingCatalog returns the one fragment projection owned by this receipt.
// A caller cannot construct an equivalent catalog or obtain one from a
// different Schema authority.
func (receipt CompilationReceipt) BindingCatalog() (BindingCatalog, bool) {
	if !receipt.Available() || receipt.catalog == nil {
		return BindingCatalog{}, false
	}
	return BindingCatalog{catalog: receipt.catalog}, true
}

func (catalog BindingCatalog) Value() *valueowner.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.value
}

func (catalog BindingCatalog) Call() *callowner.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.call
}

func (catalog BindingCatalog) Heap() *heapowner.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.heap
}

func (catalog BindingCatalog) Pack() *packowner.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.pack
}

func (catalog BindingCatalog) Effect() *effectowner.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.effect
}

func (catalog BindingCatalog) ValueSource() *valuesource.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueSource
}

func (catalog BindingCatalog) PackSource() *packsource.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.packSource
}

func (catalog BindingCatalog) HeapIngress() *heapingress.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.heapIngress
}

func (catalog BindingCatalog) ValueAllocation() *valueallocation.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueAllocation
}

func (catalog BindingCatalog) HeapEmpty() *heapempty.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.heapEmpty
}

func (catalog BindingCatalog) HeapClosed() *heapclosed.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.heapClosed
}

func (catalog BindingCatalog) RawGet() *heapindex.RawGetSchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.rawGet
}

func (catalog BindingCatalog) RawSet() *heapindex.RawSetSchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.rawSet
}

func (catalog BindingCatalog) CallDispatch() *calldispatch.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.callDispatch
}

func (catalog BindingCatalog) EffectSelected() *callsite.SelectedSchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.effectSelected
}

func (catalog BindingCatalog) EffectOpaque() *callsite.OpaqueSchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.effectOpaque
}

func (catalog BindingCatalog) EffectBody() *callsite.BodySchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.effectBody
}

func (catalog BindingCatalog) CallActivation() *callactivation.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.callActivation
}

func (catalog BindingCatalog) ValueBootstrap() *valuebootstrap.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueBootstrap
}

func (catalog BindingCatalog) HeapBootstrap() *heapbootstrap.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.heapBootstrap
}

func (catalog BindingCatalog) ValueTransfer() *valuetransfer.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueTransfer
}

func (catalog BindingCatalog) ValueArithmetic() *valuearithmetic.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueArithmetic
}

func (catalog BindingCatalog) ValueEquality() *valueequality.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueEquality
}

func (catalog BindingCatalog) ValueOrder() *valueorder.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueOrder
}

func (catalog BindingCatalog) ValueRefinement() *valuerefinement.SchemaFragment {
	if catalog.catalog == nil {
		return nil
	}
	return catalog.catalog.valueRefinement
}

func (receipt CompilationReceipt) Available() bool {
	return receipt.catalog != nil && receipt.catalog.schema != nil && receipt.catalog.schema.Available() && receipt.digest.Available() && receipt.version == ReceiptVersion
}

func (receipt CompilationReceipt) Digest() keyspace.ContentID {
	if !receipt.Available() {
		return keyspace.ContentID{}
	}
	return receipt.digest
}

func (receipt CompilationReceipt) Version() uint64 {
	if !receipt.Available() {
		return 0
	}
	return receipt.version
}

// Schema is intentionally available only to sibling internal compiler code;
// the receipt itself remains the authority fence.
func (receipt CompilationReceipt) Schema() *engine.Schema {
	if !receipt.Available() {
		return nil
	}
	return receipt.catalog.schema
}

// QueryViews is the immutable typed projection needed by a later hot binder.
// It exposes no callbacks or mutable catalog state.
type QueryViews struct {
	valueQuery  *engine.QuerySlot[programquery.ValueSummaryObservation]
	valueRead   engine.SchemaReadForm[valuedomain.Value]
	effectQuery *engine.QuerySlot[programquery.EffectObservation]
	effectRead  engine.SchemaReadForm[effectfactor.Value]
}

func (receipt CompilationReceipt) Queries() (QueryViews, bool) {
	if !receipt.Available() || receipt.catalog.valueQuery == nil || receipt.catalog.effectQuery == nil {
		return QueryViews{}, false
	}
	return QueryViews{valueQuery: receipt.catalog.valueQuery, valueRead: receipt.catalog.valueRead, effectQuery: receipt.catalog.effectQuery, effectRead: receipt.catalog.effectRead}, true
}

func (views QueryViews) Value() (*engine.QuerySlot[programquery.ValueSummaryObservation], engine.SchemaReadForm[valuedomain.Value], bool) {
	return views.valueQuery, views.valueRead, views.valueQuery != nil && views.valueRead.Schema() != nil
}

func (views QueryViews) Effect() (*engine.QuerySlot[programquery.EffectObservation], engine.SchemaReadForm[effectfactor.Value], bool) {
	return views.effectQuery, views.effectRead, views.effectQuery != nil && views.effectRead.Schema() != nil
}

var global struct {
	once    sync.Once
	receipt CompilationReceipt
	ok      bool
}

func Global() (CompilationReceipt, bool) {
	global.once.Do(func() { global.receipt, global.ok = build() })
	return global.receipt, global.ok
}

func build() (CompilationReceipt, bool) {
	v, ok := semanticvocabulary.New()
	if !ok {
		return CompilationReceipt{}, false
	}
	builder := engine.NewSchema()
	value, ok := valueowner.DeclareSchema(builder, v.ValueFactor, v.ValueSummary)
	if !ok {
		return CompilationReceipt{}, false
	}
	call, ok := callowner.DeclareSchema(builder, v.CallFactor)
	if !ok {
		return CompilationReceipt{}, false
	}
	heap, ok := heapowner.DeclareSchema(builder, v.HeapFactor)
	if !ok {
		return CompilationReceipt{}, false
	}
	pack, ok := packowner.DeclareSchema(builder, v.PackFactor)
	if !ok {
		return CompilationReceipt{}, false
	}
	effect, ok := effectowner.DeclareSchema(builder, v.EffectFactor)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueSource, ok := valuesource.DeclareSchema(builder, v.ValueSourceRule.Rule, v.ValueSourceRule.Operand, v.ValueSourceRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	packSource, ok := packsource.DeclareSchema(builder, v.PackSourceRule.Rule, v.PackSourceRule.Operand, v.PackSourceRule.Evidence, pack)
	if !ok {
		return CompilationReceipt{}, false
	}
	heapIngress, ok := heapingress.DeclareSchema(builder, v.HeapIngressRule.Rule, v.HeapIngressRule.Operand, v.HeapIngressRule.Evidence, heap)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueAllocation, ok := valueallocation.DeclareSchema(builder, v.ValueAllocationRule.Rule, v.ValueAllocationRule.Operand, v.ValueAllocationRule.Transform, v.ValueAllocationRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	heapEmpty, ok := heapempty.DeclareSchema(builder, v.HeapEmptyRule.Rule, v.HeapEmptyRule.Operand, v.HeapEmptyRule.Transform, v.HeapEmptyRule.Evidence, heap)
	if !ok {
		return CompilationReceipt{}, false
	}
	heapClosed, ok := heapclosed.DeclareSchema(builder, v.HeapClosedRule.Rule, v.HeapClosedRule.Operand, v.HeapClosedRule.Transform, v.HeapClosedRule.Evidence, heap, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	rawGet, ok := heapindex.DeclareRawGetSchema(builder, v.RawGetRule.Rule, v.RawGetRule.Operand, v.RawGetRule.Evidence, value, call, heap, pack)
	if !ok {
		return CompilationReceipt{}, false
	}
	rawSet, ok := heapindex.DeclareRawSetSchema(builder, v.RawSetRule.Rule, v.RawSetRule.Operand, v.RawSetRule.Evidence, value, heap, pack)
	if !ok {
		return CompilationReceipt{}, false
	}
	callDispatch, ok := calldispatch.DeclareSchema(builder, v.CallDispatchRule.Rule, v.CallDispatchRule.Operand, v.CallDispatchRule.Evidence, value, call)
	if !ok {
		return CompilationReceipt{}, false
	}
	effectSelected, ok := callsite.DeclareSelectedSchema(builder, v.EffectSelectedRule.Rule, v.EffectSelectedRule.Operand, v.EffectSelectedRule.Evidence, call, effect)
	if !ok {
		return CompilationReceipt{}, false
	}
	effectOpaque, ok := callsite.DeclareOpaqueSchema(builder, v.EffectOpaqueRule.Rule, v.EffectOpaqueRule.Operand, v.EffectOpaqueRule.Evidence, call, effect)
	if !ok {
		return CompilationReceipt{}, false
	}
	effectBody, ok := callsite.DeclareBodySchema(builder, v.EffectBodyRule.Rule, v.EffectBodyRule.Operand, v.EffectBodyRule.Evidence, call, effect)
	if !ok {
		return CompilationReceipt{}, false
	}
	callActivation, ok := callactivation.DeclareSchema(builder, v.CallActivation, v.CallActivationFamily, v.CallActivationAdmission, call)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueBootstrap, ok := valuebootstrap.DeclareSchema(builder, v.ValueBootstrapRule.Rule, v.ValueBootstrapRule.Operand, v.ValueBootstrapRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	heapBootstrap, ok := heapbootstrap.DeclareSchema(builder, v.HeapBootstrapRule.Rule, v.HeapBootstrapRule.Operand, v.HeapBootstrapRule.Evidence, heap)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueTransfer, ok := valuetransfer.DeclareSchema(builder, v.ValueTransferRule.Rule, v.ValueTransferRule.Operand, v.ValueTransferRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueArithmetic, ok := valuearithmetic.DeclareSchema(builder, v.ValueBinaryArithmeticRule.Rule, v.ValueBinaryArithmeticRule.Operand, v.ValueBinaryArithmeticRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueEquality, ok := valueequality.DeclareSchema(builder, v.ValueBinaryEqualityRule.Rule, v.ValueBinaryEqualityRule.Operand, v.ValueBinaryEqualityRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueOrder, ok := valueorder.DeclareSchema(builder, v.ValueBinaryOrderRule.Rule, v.ValueBinaryOrderRule.Operand, v.ValueBinaryOrderRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	valueRefinement, ok := valuerefinement.DeclareSchema(builder, v.ValuePresenceRefinementRule.Rule, v.ValuePresenceRefinementRule.Operand, v.ValuePresenceRefinementRule.Evidence, value)
	if !ok {
		return CompilationReceipt{}, false
	}
	// Query payloads are deliberately marker-free schema slots. Their typed
	// hot projectors belong to analysis binding, while these two cold query
	// identities remain part of this sole schema owner.
	valueRead := value.SummaryRead()
	effectRead := effect.ExactRead()
	if valueRead.Schema() != nil || effectRead.Schema() != nil {
		return CompilationReceipt{}, false
	}
	valueQuery, ok := engine.NewQuerySlot[programquery.ValueSummaryObservation](builder, engine.SchemaQuerySpec{Semantic: v.ValueQuery, Freezer: v.ValueCodec})
	if !ok || !engine.SchemaQueryRead(valueQuery, valueRead) {
		return CompilationReceipt{}, false
	}
	effectQuery, ok := engine.NewQuerySlot[programquery.EffectObservation](builder, engine.SchemaQuerySpec{Semantic: v.EffectQuery, Freezer: v.EffectCodec})
	if !ok || !engine.SchemaQueryRead(effectQuery, effectRead) {
		return CompilationReceipt{}, false
	}
	schema, ok := builder.Seal()
	if !ok || schema == nil || !schema.Available() {
		return CompilationReceipt{}, false
	}
	digest := keyspace.ContentID(schema.ID().Digest())
	if !digest.Available() {
		return CompilationReceipt{}, false
	}
	receipt := CompilationReceipt{
		catalog: &catalog{
			schema:          schema,
			value:           value,
			call:            call,
			heap:            heap,
			pack:            pack,
			effect:          effect,
			valueSource:     valueSource,
			packSource:      packSource,
			heapIngress:     heapIngress,
			valueAllocation: valueAllocation,
			heapEmpty:       heapEmpty,
			heapClosed:      heapClosed,
			rawGet:          rawGet,
			rawSet:          rawSet,
			callDispatch:    callDispatch,
			effectSelected:  effectSelected,
			effectOpaque:    effectOpaque,
			effectBody:      effectBody,
			callActivation:  callActivation,
			valueBootstrap:  valueBootstrap,
			heapBootstrap:   heapBootstrap,
			valueTransfer:   valueTransfer,
			valueArithmetic: valueArithmetic,
			valueEquality:   valueEquality,
			valueOrder:      valueOrder,
			valueRefinement: valueRefinement,
			valueQuery:      valueQuery,
			valueRead:       valueRead,
			effectQuery:     effectQuery,
			effectRead:      effectRead,
		},
		digest:  digest,
		version: ReceiptVersion,
	}
	return receipt, receipt.Available()
}
