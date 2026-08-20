package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
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

// ValueQuery is the sealed implementation of the value-summary family. The
// fragment remains the canonical row; the implementation is projected from
// that row against this exact sealed binding.
func (bound *ProgramBinding) ValueQuery() *valueowner.SummaryQueryImplementation {
	cell, ok := bound.Query(QueryFamilyValueSummary)
	if !ok {
		return nil
	}
	fragment, present := query.Payload[*valueowner.SummaryQueryFragment](cell)
	if !present {
		return nil
	}
	implementation, recovered := valueowner.RecoverQuery(bound.binding, query.Sealed[*valueowner.SummaryQueryFragment]{Fragment: fragment})
	if !recovered {
		return nil
	}
	return implementation
}

// EffectQuery is the sealed implementation of the effect-exact family.
func (bound *ProgramBinding) EffectQuery() *effectowner.ExactQueryImplementation {
	cell, ok := bound.Query(QueryFamilyEffectExact)
	if !ok {
		return nil
	}
	fragment, present := query.Payload[*effectowner.ExactQueryFragment](cell)
	if !present {
		return nil
	}
	implementation, recovered := effectowner.RecoverQuery(bound.binding, query.Sealed[*effectowner.ExactQueryFragment]{Fragment: fragment})
	if !recovered {
		return nil
	}
	return implementation
}

// QueryAdmission seals one selected-point query row from the family's own
// implementation. Construction walks sealed issuance for sites; the sealed
// family selects its owner's admission callback. Projection is descriptive
// query geometry and is never used as a concrete implementation type tag.
func (bound *ProgramBinding) QueryAdmission(id, mount, point identity.ContentID, family schema.Key) (engine.ProgramQueryAdmission, bool) {
	if bound == nil {
		return engine.ProgramQueryAdmission{}, false
	}
	position, ok := queryPositionForFamily(family)
	if !ok || position < 0 || position >= len(registry.queryContributors) {
		return engine.ProgramQueryAdmission{}, false
	}
	cell, ok := bound.Query(family)
	if !ok {
		return engine.ProgramQueryAdmission{}, false
	}
	return registry.queryContributors[position].admit(bound.binding, cell, id, mount, point)
}

// BindProgram binds the complete global schema in one SchemaBinding. The
// caller supplies the one immutable compilation handle obtained at the
// composition root and the record the mount phase produced; the factor
// principals, allocation catalog, rules, and query families are admitted by the
// grammar's own transaction.
func BindProgram(compilation Compilation, inputs LinkInputs) (*ProgramBinding, BindFailure) {
	bound, failure := bind(compilation, inputs)
	if failure.Available() {
		return nil, failure
	}
	return &ProgramBinding{
		binding:     bound.binding,
		rules:       bound.rules,
		allocations: bound.allocations,
		value:       bound.value,
		queries:     bound.queries,
	}, BindFailure{}
}
