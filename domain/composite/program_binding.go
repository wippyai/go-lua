package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
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

	// queries holds every declared query family's sealed implementation at its
	// slot, opaque here and recovered at its type by the accessor the family's
	// own consumers read it through.
	queries queryCells
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

// Query recovers the sealed implementation cell of one issued family. The
// cell stays opaque here; the caller recovers it at the type the family's
// own contributor declared.
func (bound *ProgramBinding) Query(family schema.Key) (query.Cell, bool) {
	if bound == nil || !family.Available() {
		return query.Cell{}, false
	}
	position, ok := queryPositionForFamily(family)
	if !ok || position < 0 || position >= len(bound.queries) {
		return query.Cell{}, false
	}
	cell := bound.queries[position]
	return cell, cell.Available()
}

// ValueQuery is the receipt-native implementation of the value-summary family.
// It recovers the family's own cell at the type its contributor declared, so
// the accessor names the family and never a fold of its own.
func (bound *ProgramBinding) ValueQuery() *valueowner.SummaryQueryImplementation {
	cell, ok := bound.Query(QueryFamilyValueSummary)
	if !ok {
		return nil
	}
	implementation, recovered := query.Payload[*valueowner.SummaryQueryImplementation](cell)
	if !recovered {
		return nil
	}
	return implementation
}

// EffectQuery is the receipt-native implementation of the effect-exact family.
func (bound *ProgramBinding) EffectQuery() *effectowner.ExactQueryImplementation {
	cell, ok := bound.Query(QueryFamilyEffectExact)
	if !ok {
		return nil
	}
	implementation, recovered := query.Payload[*effectowner.ExactQueryImplementation](cell)
	if !recovered {
		return nil
	}
	return implementation
}

// QueryAdmission seals one selected-point query row from the family's own
// implementation. Construction walks sealed issuance for sites; this recovers
// the cell by projection.
func (bound *ProgramBinding) QueryAdmission(id, mount, point identity.ContentID, projection schema.Key) (engine.ProgramQueryAdmission, bool) {
	if bound == nil {
		return engine.ProgramQueryAdmission{}, false
	}
	switch projection {
	case query.ProjectionSummary:
		return engine.NewSummaryQueryAdmission(bound.ValueQuery(), id, mount, point)
	case query.ProjectionExact:
		return engine.NewExactQueryAdmission(bound.EffectQuery(), id, mount, point)
	default:
		return engine.ProgramQueryAdmission{}, false
	}
}

// AttachQuery binds one selected-point query through the open construction
// and returns the publication key Result reads Snapshot by.
func (bound *ProgramBinding) AttachQuery(compilation *engine.ProgramConstruction, id identity.ContentID, projection schema.Key) (identity.ContentID, bool) {
	if bound == nil || compilation == nil {
		return identity.ContentID{}, false
	}
	var ok bool
	switch projection {
	case query.ProjectionSummary:
		ok = engine.AttachSummaryQuery(compilation, bound.ValueQuery(), id)
	case query.ProjectionExact:
		ok = engine.AttachExactQuery(compilation, bound.EffectQuery(), id)
	}
	if !ok {
		return identity.ContentID{}, false
	}
	return compilation.QueryPublicationKey(id)
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

// BindProgram binds the complete global schema in one SchemaBinding. The factor
// principals, the allocation catalog, every rule, and every declared query
// family are admitted by the grammar's own transaction; the caller supplies the
// record the mount phase produced and receives the sealed hot binding.
func BindProgram(compilation Compilation, inputs LinkInputs) (*ProgramBinding, BindFailure) {
	bound, failure := bind(compilation, inputs)
	if failure.Available() {
		return nil, failure
	}
	runtimeContexts, runtimeContextsOK := newRuntimeAllocationContextOwner(inputs.PackSchema, inputs.HeapSchema)
	if !runtimeContextsOK {
		return nil, BindFailure{Stage: BindStageRuntimeContexts}
	}
	return &ProgramBinding{
		binding:         bound.binding,
		rules:           bound.rules,
		allocations:     bound.allocations,
		value:           bound.value,
		runtimeContexts: runtimeContexts,
		queries:         bound.queries,
	}, BindFailure{}
}
