package composite

import (
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ProgramBinding is the one sealed Link-local hot binding for the global
// reusable Program schema. Compile constructs it once; repeated and concurrent
// Plan solves reuse its immutable typed cells while creating independent
// solve-local topology/runtime transactions.
//
// The rule plane is not enumerated here: the binding transaction drives the
// sealed rule table and this record retains only its neutral hot projection.
type ProgramBinding struct {
	binding *engine.SchemaBinding

	// rules is the sealed rule table's Link-local projection. Every rule
	// admission, attachment, and classification goes through it.
	rules *RuleBinding

	// allocations is the Link-owned allocation directory admitted with this
	// binding.
	allocations *allocationcatalog.Catalog

	value *valueowner.HotOwner

	// runtimeContexts retains only the already-sealed Pack/Heap authorities
	// needed to join an externally-issued runtime context later. It owns no
	// runtime policy or live capability: those remain runtime inputs and are
	// revoked by their authority, not by compilation.
	runtimeContexts RuntimeAllocationContextOwner

	valueQuery  *engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation]
	effectQuery *engine.ExactQueryImplementation[effectfactor.Value, effectfactor.EffectObservation]
}

// Available states that this binding completed its transaction and sealed.
func (bound *ProgramBinding) Available() bool {
	return bound != nil && bound.binding != nil && bound.binding.Sealed() &&
		bound.rules != nil && bound.allocations != nil
}

// SchemaBinding is the one sealed engine binding this transaction produced.
func (bound *ProgramBinding) SchemaBinding() *engine.SchemaBinding {
	if bound == nil {
		return nil
	}
	return bound.binding
}

// Rules is the hot projection of the sealed rule table.
func (bound *ProgramBinding) Rules() *RuleBinding {
	if bound == nil {
		return nil
	}
	return bound.rules
}

// ValueSchema is the sealed value schema this binding's principal carries. The
// principal itself stays inside the binding.
func (bound *ProgramBinding) ValueSchema() *valuedomain.Schema {
	if bound == nil || bound.value == nil {
		return nil
	}
	return bound.value.Schema()
}

// ValueQuery is the receipt-native implementation of the value summary query.
func (bound *ProgramBinding) ValueQuery() *engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation] {
	if bound == nil {
		return nil
	}
	return bound.valueQuery
}

// EffectQuery is the receipt-native implementation of the exact effect query.
func (bound *ProgramBinding) EffectQuery() *engine.ExactQueryImplementation[effectfactor.Value, effectfactor.EffectObservation] {
	if bound == nil {
		return nil
	}
	return bound.effectQuery
}

// RuntimeContexts is the cold authority pair a runtime joins to open allocation
// contexts against this exact binding.
func (bound *ProgramBinding) RuntimeContexts() RuntimeAllocationContextOwner {
	if bound == nil {
		return RuntimeAllocationContextOwner{}
	}
	return bound.runtimeContexts
}

// RuntimeAllocationContextOwner is Plan-local cold authority for creating a
// Pack/Heap issuer after a runtime supplies its policy identity. Keeping this
// pair in the binding makes a positive publication proof join the exact schemas
// that produced it; a separate equal-content reseal cannot enter through
// matching scalar IDs.
type RuntimeAllocationContextOwner struct {
	pack *packdomain.Schema
	heap heapdomain.Schema
}

func newRuntimeAllocationContextOwner(packSchema *packdomain.Schema, heapSchema heapdomain.Schema) (RuntimeAllocationContextOwner, bool) {
	owner := RuntimeAllocationContextOwner{pack: packSchema, heap: heapSchema}
	return owner, owner.Valid()
}

func (owner RuntimeAllocationContextOwner) Valid() bool {
	return owner.pack != nil && owner.heap.Valid() && owner.heap.LinkOwner().Matches(owner.pack.LinkOwner())
}

// Pack is the sealed pack schema this owner joins against.
func (owner RuntimeAllocationContextOwner) Pack() *packdomain.Schema { return owner.pack }

// Heap is the sealed heap schema this owner joins against.
func (owner RuntimeAllocationContextOwner) Heap() heapdomain.Schema { return owner.heap }

// Begin joins a runtime-owned policy identity to this exact Plan's sealed
// Pack/Heap pair. The returned authority remains short-lived and must be
// closed by the runtime owner; neither this Plan-local owner nor Result keeps
// a live context capability.
func (owner RuntimeAllocationContextOwner) Begin(policyID identity.ContentID) (*heapdomain.RuntimeAllocationContextAuthority, packdomain.RuntimeAllocationContextBindingIssuer, bool) {
	if !owner.Valid() {
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

// ProgramQuerySpecs is the caller-owned hot query surface installed on the
// value and effect principals during the binding transaction. The catalog
// declares the query slots and their semantic keys; the fold and freeze
// behaviour behind them stays the analyzer's.
type ProgramQuerySpecs struct {
	Value  engine.HotSummaryQuerySpec[valuedomain.Value, valuedomain.ValueSummaryObservation]
	Effect engine.HotExactQuerySpec[effectfactor.Value, effectfactor.EffectObservation]
}

// BindProgram binds the complete global schema in one SchemaBinding. The factor
// principals, the allocation catalog, and every rule are admitted by the
// grammar's own transaction; the caller supplies the record the mount phase
// produced and its own query specs, and receives the sealed hot binding.
func BindProgram(compilation Compilation, inputs LinkInputs, queries ProgramQuerySpecs) (*ProgramBinding, BindFailure) {
	var (
		value           *valueowner.HotOwner
		valueQuerySlot  *engine.QuerySlot[valuedomain.ValueSummaryObservation]
		effectQuerySlot *engine.QuerySlot[effectfactor.EffectObservation]
	)
	bound, failure := bind(compilation, inputs, func(valueOwner *valueowner.HotOwner, effectOwner *effectowner.HotOwner, views QueryViews) bool {
		valueQuery, valueRead, valueViewOK := views.Value()
		effectQuery, _, effectViewOK := views.Effect()
		if !valueViewOK || !effectViewOK {
			return false
		}
		if !valueowner.BindSummaryQuery(valueOwner, valueQuery, valueRead, queries.Value) {
			return false
		}
		if !effectowner.BindExactQuery(effectOwner, effectQuery, queries.Effect) {
			return false
		}
		value = valueOwner
		valueQuerySlot, effectQuerySlot = valueQuery, effectQuery
		return true
	})
	if failure.Available() {
		return nil, failure
	}
	valueQuery, ok := engine.SummaryQueryImplementationAt[valuedomain.Value, valuedomain.ValueSummaryObservation](bound.binding, valueQuerySlot)
	if !ok {
		return nil, BindFailure{Stage: BindStageValueQueryReceipt}
	}
	effectQuery, ok := engine.ExactQueryImplementationAt[effectfactor.Value, effectfactor.EffectObservation](bound.binding, effectQuerySlot)
	if !ok {
		return nil, BindFailure{Stage: BindStageEffectQueryReceipt}
	}
	runtimeContexts, runtimeContextsOK := newRuntimeAllocationContextOwner(inputs.PackSchema, inputs.HeapSchema)
	if !runtimeContextsOK {
		return nil, BindFailure{Stage: BindStageRuntimeContexts}
	}
	return &ProgramBinding{
		binding:         bound.binding,
		rules:           bound.rules,
		allocations:     bound.allocations,
		value:           value,
		runtimeContexts: runtimeContexts,
		valueQuery:      valueQuery,
		effectQuery:     effectQuery,
	}, BindFailure{}
}
