// Package snapshot is the published value of the analysis engine. A Snapshot
// is what a consumer holds: an immutable, atomically published view of one
// store at one generation. The engine owns the mutable side and mutates it
// only inside an epoch; publication is the only way engine state becomes
// consumable, and a consumer never holds anything but snapshots.
//
// # Layering
//
// This package imports only the identity leaf and the standard library. It
// has no domain vocabulary, no schema vocabulary, and no engine types. An
// executable import law enforces that, so a later move cannot quietly turn the
// published value into a second engine surface.
//
// # Addressing
//
// An Axis names storage; it never contains it. Axis[K, V] carries the sealing
// schema's ContentID and a dense column slot, and the K and V parameters are
// the static claim about what lives in that column. Snapshot stores its
// columns type-erased, so no read path exists that boxes a key or a value.
// Reads are generic free functions instead of methods, because a method on a
// non-generic Snapshot would have to erase K and V again to be callable.
//
// Read validates the schema, the slot bound, and the column kind before it
// recovers the typed column pointer, and every failure returns ReadInvalid
// rather than a status a caller could mistake for an answer. Kind validation
// is the checked typed recovery itself: recovering a *column[K, V] out of the
// erased slot succeeds only when the sealed column was built for exactly that
// key and value type, and a mismatch fails closed.
//
// # Read outcomes
//
// A valid read reports one of three outcomes. ReadHit carries the stored
// value. ReadProvenAbsent states that the column's sealed denominator covers
// the key and the column has no row for it, so absence is a fact rather than
// ignorance. ReadMiss states only that this column has no row for the key,
// which is what a column without a denominator can ever say. The distinction
// is load bearing: a proven absence is a semantic conclusion, a miss is not.
//
// # Storage
//
// Every published structure -- a column's rows, a denominator's membership,
// the directory, and the query publication -- is stored in one persistent
// hash trie. The shape follows from the cost of publishing: an engine
// publishes a new snapshot whenever a fact changes, so publication has to be
// priced by the change set rather than by the state. A trie whose nodes are
// shared by pointer and copied only along a changed key's path prices it that
// way: publishing d changed rows copies the nodes on d paths, path length is
// bounded by the hash width rather than by the row count, and every untouched
// column, untouched denominator and untouched node is the very structure the
// previous publication holds.
//
// The alternatives fail that price. Sharing a flat mapping publishes a
// structure the engine can still write, and copying one prices every
// publication at the size of the state. A fixed chunk table shares what it
// did not touch, but with a fixed chunk count the chunk grows with the
// column, so one changed row again copies a share of the whole.
//
// A key is hashed by what makes two keys equal: the schedule is derived once
// per key type, hashes a string by its contents, normalizes the two zeros of
// a float, and never hashes a struct's padding. Flat keys -- an integer, a
// content identity, a struct of scalars -- coalesce into a single memory
// pass, so a read hashes once and then walks a few nodes.
//
// # Denominators
//
// Membership is a value with its own identity, sealed once and referenced by
// every column that is total over it. The first column that names a
// denominator identity declares its members; a later column names the
// identity alone and is sealed against that very set. Two columns total over
// one key universe therefore cost one membership set, and no column edit can
// reach a sealed one.
//
// # Publication
//
// A Builder accumulates a publication and Seal consumes it. NewBuilder starts
// from nothing. NewDelta starts from a sealed snapshot and inherits its
// columns, directory, denominators and query publication by reference, so a
// derived publication states its change set: SetRow publishes a row,
// RemoveRow withdraws one, and PutColumn reseals a whole slot. A derived
// publication must advance the generation of the store it derives from,
// because two snapshots of one store at one generation would make one
// locator address two different contents.
//
// # Queries
//
// A query family is answered by a result column: the same storage, addressed
// under the family's identity and registered as answerable. DeclareQuery
// writes those three facts together and returns the QueryPlan a consumer
// reads; OpenQuery hands a consumer the plan the snapshot itself published,
// and refuses a family that is not registered, not addressed, or answered by
// a column of other types. Query reports the same four outcomes a column read
// reports, so a materialized absence stays distinguishable from ignorance,
// and a materializer publishes and withdraws answers through the ordinary row
// edits.
//
// # Locators
//
// Resolve turns a ContentID into at most one Locator through the snapshot's
// immutable directory. A Locator is a snapshot-relative address, not an
// identity: it is valid only against the snapshot, store, and generation that
// issued it, and it carries an unexported coordinate so no consumer can mint
// one, persist one, or read one back out as a durable key. Cross-store
// semantic identity is ContentID and nothing else.
//
// # Borrowed values
//
// Values read out of a snapshot are borrowed and transitively immutable. The
// package returns values, never column internals, and exports no way to name
// a column. For a value shape that carries a reference, Go cannot enforce the
// rest: the owner that published the value is obliged to publish only
// structures it will never write again, and a consumer that needs a mutable
// form detaches its own copy. Detachment is never charged to a borrowed read.
//
// # Cost
//
// Point reads, queries and Resolve allocate nothing, enforced by allocation
// laws. A publication costs its change set, enforced by a measured delta law.
// A Snapshot is a value: copying one shares the published structure and
// copies no rows, so snapshots and Locators survive assignment, slice
// placement, and map placement unchanged.
package snapshot
