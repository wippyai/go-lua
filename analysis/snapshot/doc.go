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
// Point reads and Resolve allocate nothing, enforced by allocation laws. A
// Snapshot is a value: copying one shares the published structure and copies
// no rows, so snapshots and Locators survive assignment, slice placement, and
// map placement unchanged.
package snapshot
