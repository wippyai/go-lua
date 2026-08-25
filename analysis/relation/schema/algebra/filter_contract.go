package algebra

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// SelectMode chooses the closed Select contract form. Semantic predicates are
// deliberately absent: Apply is the only operation boundary, and equality or
// tag conditions normalize to Join over declared columns.
type SelectMode uint8

const (
	SelectModeInvalid SelectMode = iota
	SelectByScope
)

// SelectContract is an immutable declarative filter contract over closed
// schema facts. It stores only the nominal ScopeID; the canonical ScopeSchema
// is resolved from ExecutionSchema. Executable predicates are represented by
// an explicit Apply expression instead of a second callback or operation-token
// mechanism.
type SelectContract struct {
	mode  SelectMode
	scope model.ScopeID
}

// NewSelectContract constructs a scope filter without semantic validation.
// Scope membership and dimensions are checker concerns owned by the sealed
// ExecutionSchema registry.
func NewSelectContract(mode SelectMode, scope model.ScopeID) SelectContract {
	return SelectContract{mode: mode, scope: scope}
}

// Mode returns the declared filter mode.
func (contract SelectContract) Mode() SelectMode { return contract.mode }

// Scope returns the nominal scope identity. The checker resolves its schema
// from ExecutionSchema.Scopes.
func (contract SelectContract) Scope() model.ScopeID { return contract.scope }

func (contract SelectContract) digestBytes() []byte {
	parts := appendUint8(nil, uint8(contract.mode))
	parts = appendScopeID(parts, contract.scope)
	return parts
}

// ColumnMapping declares one logical source-to-target column correspondence.
// It carries no physical ordinal or computation callback.
type ColumnMapping struct {
	source model.ColumnID
	target model.ColumnID
}

// NewColumnMapping constructs a mapping; ownership and relation compatibility
// are checker concerns and are intentionally not enforced here.
func NewColumnMapping(source, target model.ColumnID) ColumnMapping {
	return ColumnMapping{source: source, target: target}
}

// Source returns the mapped source column.
func (mapping ColumnMapping) Source() model.ColumnID { return mapping.source }

// Target returns the mapped target column.
func (mapping ColumnMapping) Target() model.ColumnID { return mapping.target }

func (mapping ColumnMapping) digestBytes() []byte {
	parts := appendColumn(nil, mapping.source)
	return appendColumn(parts, mapping.target)
}

// ProjectContract declares target relation, authored mappings, and output key.
// Mapping order is retained as part of the typed row contract.
type ProjectContract struct {
	target   model.RelationID
	mappings []ColumnMapping
	key      model.KeyID
}

// NewProjectContract copies mappings and leaves semantic compatibility to the
// checker.
func NewProjectContract(target model.RelationID, mappings []ColumnMapping, key model.KeyID) ProjectContract {
	return ProjectContract{target: target, mappings: cloneMappings(mappings), key: key}
}

// Target returns the projected relation identity.
func (contract ProjectContract) Target() model.RelationID { return contract.target }

// Mappings returns a defensive copy in authored order.
func (contract ProjectContract) Mappings() []ColumnMapping { return cloneMappings(contract.mappings) }

// Key returns the projected output key.
func (contract ProjectContract) Key() model.KeyID { return contract.key }

func (contract ProjectContract) digestBytes() []byte {
	parts := appendRelation(nil, contract.target)
	parts = appendLength(parts, len(contract.mappings))
	for _, mapping := range contract.mappings {
		parts = appendBytes(parts, mapping.digestBytes())
	}
	return appendKey(parts, contract.key)
}
