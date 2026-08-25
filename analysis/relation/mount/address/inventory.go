package address

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// Inventory is the physical mount boundary consumed by Bind.  An inventory
// owns the interpretation of its private uint64 coordinates; address only
// snapshots the returned coordinates into generation-fenced Locators.
//
// Implementations must return the same Fence for the lifetime of one Bind
// call.  Bind calls each resolver exactly once for each certified logical ID,
// then never consults the inventory again.
type Inventory interface {
	Fence() Fence
	ResolveRelation(model.RelationID) (uint64, bool)
	ResolveColumn(model.ColumnID) (uint64, bool)
	ResolveKey(model.KeyID) (uint64, bool)
	ResolveScope(model.ScopeID) (uint64, bool)
	ResolveExpression(model.ExpressionID) (uint64, bool)
	ResolveDependency(model.DependencyID) (uint64, bool)
}
