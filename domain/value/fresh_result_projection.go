package value

import (
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

// FreshResultAtom projects the presealed Value reference for one Target fresh
// result.  Fresh roots share Heap's allocation-key family with authored
// Program allocations, so the FreshResultID check is intentional: a valid
// RootAllocation key alone is not sufficient to enter this authority.
//
// Recent and Summary are the only materialization roles that represent a
// fresh result. Exact is the raw allocation-reference alternative and must
// remain in the Value stored-unknown partition until the owning contribution
// admits its Recent result. No Target or coordinate lookup is performed.
func (schema *Schema) FreshResultAtom(key heap.Key, role materialization.Role) (Atom, bool) {
	if schema == nil || !schema.heap.Valid() || !schema.heap.OwnsKey(key) ||
		key.Kind() != heap.RootAllocation ||
		(role != materialization.Recent && role != materialization.Summary) {
		return Atom{}, false
	}
	application, outcomeResult, _, fresh := key.FreshResultID()
	if !fresh || !application.Available() || !outcomeResult.Available() {
		return Atom{}, false
	}

	reference := schema.allocRefs[key]
	if reference == 0 || int(reference) > len(schema.references) {
		return Atom{}, false
	}
	row := schema.references[reference-1]
	if row.source != referenceSourceAllocation || row.allocation != key {
		return Atom{}, false
	}
	id, ok := schema.referenceAtom(reference, role)
	if !ok {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

// FreshResultFact returns the exact singleton Value for one fresh-result
// materialization role. The atom and its owner are issued by FreshResultAtom;
// this facade adds no coordinate, Target, or Placement state.
func (schema *Schema) FreshResultFact(key heap.Key, role materialization.Role) (Value, bool) {
	atom, ok := schema.FreshResultAtom(key, role)
	if !ok {
		return Value{}, false
	}
	return schema.Singleton(atom)
}
