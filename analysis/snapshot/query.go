package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// QueryPlan names one published result column: the schema that sealed it and
// the dense slot its answers occupy. It is the reading half of a query
// family, and it is an address rather than an identity in exactly the sense
// an Axis is: it names storage of one schema and holds none.
//
// K and O are the static claim about a family's key and its answer. A query
// carries that claim to the snapshot, which either recovers a result column
// built for exactly those types or fails closed.
//
// A result column is a column. It is stored, addressed, edited and published
// through the same storage as an axis column, which is why a plan converts to
// the Axis that addresses it and why a materializer publishes answers with
// the ordinary row edits rather than a second write path.
type QueryPlan[K comparable, O any] Axis[K, O]

// Available reports whether plan names a result column of a sealed schema.
// The zero QueryPlan names none.
func (plan QueryPlan[K, O]) Available() bool { return plan.SchemaID.Available() }

// Axis returns the column address this plan reads and a builder edits.
func (plan QueryPlan[K, O]) Axis() Axis[K, O] { return Axis[K, O](plan) }

// Query answers key against the result column plan names. It is a read of a
// published answer, not a computation: it validates the schema, the slot
// bound and the result column's key and answer types, and reports the same
// four outcomes a column read reports, so a materialized absence stays
// distinguishable from ignorance.
//
// The returned answer is borrowed and transitively immutable. Query allocates
// nothing.
func Query[K comparable, O any](s *Snapshot, plan QueryPlan[K, O], key K) (O, ReadStatus) {
	return Read(s, Axis[K, O](plan), key)
}

// OpenQuery returns the plan this snapshot publishes for the query family
// named by family. A family the snapshot does not register, does not address,
// or answers with a result column built for other types opens nothing: a plan
// can only be obtained from the snapshot that answers it, so a consumer
// cannot mint one and read a column that was never published as an answer.
//
// OpenQuery allocates nothing.
func OpenQuery[K comparable, O any](s *Snapshot, family identity.ContentID) (QueryPlan[K, O], bool) {
	if s == nil || !s.Published() || !family.Available() {
		return QueryPlan[K, O]{}, false
	}
	if !s.queries.Published(family) {
		return QueryPlan[K, O]{}, false
	}
	slot, addressed := trieLookup(s.directory, hashKey(identityPlan, family), family)
	if !addressed {
		return QueryPlan[K, O]{}, false
	}
	if _, recovered := columnAt[K, O](&s.publication, s.schema, slot); !recovered {
		return QueryPlan[K, O]{}, false
	}
	return QueryPlan[K, O]{SchemaID: s.schema, Slot: slot}, true
}
