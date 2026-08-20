package value

import "github.com/wippyai/go-lua/analysis/identity"

// GlobalBootstrapResult is the sealed Value-owned result of one Host global
// binding. Bootstrap issues it only after validating Host, Module, Boundary,
// Target, and Value ownership; hot callbacks consume this immutable receipt.
type GlobalBootstrapResult struct {
	schema     *Schema
	bindingID  identity.ContentID
	id         identity.ContentID
	coordinate Coordinate
	fact       Value
	absent     bool
}

// NewGlobalBootstrapResult seals one fully validated bootstrap observation.
// The bootstrap package remains responsible for proving the Host mapping;
// this constructor fences the resulting Value state to the exact Schema.
func NewGlobalBootstrapResult(schema *Schema, id identity.ContentID, coordinate Coordinate, fact Value, absent bool) (*GlobalBootstrapResult, bool) {
	if schema == nil || !id.Available() || !coordinate.Valid() || coordinate.schema != schema || (!absent && (!fact.valid() || fact.schema != schema)) {
		return nil, false
	}
	return &GlobalBootstrapResult{schema: schema, bindingID: id, id: id, coordinate: coordinate, fact: fact, absent: absent}, true
}

// Owns reports exact receipt ownership without reopening Host or Boundary.
func (result *GlobalBootstrapResult) Owns(schema *Schema) bool {
	return result != nil && schema != nil && result.schema == schema && result.bindingID == result.id && result.id.Available() && result.coordinate.schema == schema && result.coordinate.Valid()
}

// ID returns the already-issued Host binding identity.
func (result *GlobalBootstrapResult) ID() (identity.ContentID, bool) {
	if result == nil || !result.id.Available() {
		return identity.ContentID{}, false
	}
	return result.id, true
}

// Coordinate returns the exact Value target paired with the binding.
func (result *GlobalBootstrapResult) Coordinate() (Coordinate, bool) {
	if result == nil || !result.coordinate.Valid() {
		return Coordinate{}, false
	}
	return result.coordinate, true
}

// Fact returns the canonical Value result for a present binding.
func (result *GlobalBootstrapResult) Fact() (Value, bool) {
	if result == nil || result.absent || !result.fact.valid() {
		return Value{}, false
	}
	return result.fact, true
}

// Absent reports the valid no-candidate InitialValueAbsent case.
func (result *GlobalBootstrapResult) Absent() bool { return result != nil && result.absent }

// GlobalBootstrapResultFor returns the receipt issued during Schema sealing.
// It performs only the exact binding-key lookup and receipt fence; all Host,
// Module, Boundary, and Target validation happened before sealing.
func (schema *Schema) GlobalBootstrapResultForID(id identity.ContentID) (*GlobalBootstrapResult, bool) {
	if schema == nil || !id.Available() || schema.globalResults == nil {
		return nil, false
	}
	result, ok := schema.globalResults[id]
	return result, ok && result.Owns(schema)
}

// GlobalBootstrapResultCount and GlobalBootstrapResultIDAt expose the exact
// sealed receipt directory. They never reopen Host globals after Value seal.
func (schema *Schema) GlobalBootstrapResultCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.globalIDs)
}

func (schema *Schema) GlobalBootstrapResultIDAt(index int) (identity.ContentID, bool) {
	if schema == nil || index < 0 || index >= len(schema.globalIDs) {
		return identity.ContentID{}, false
	}
	id := schema.globalIDs[index]
	_, ok := schema.GlobalBootstrapResultForID(id)
	return id, ok
}
