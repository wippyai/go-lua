// The Store route judgments.
//
// Which Placement coordinates a storage transfer reaches is a DECLARED
// relation: what to enumerate, how to union it, what to widen to when the
// value names no closed list of allocations, and the order the routes come
// back in are stated by the relation and written by the emitter. What is left
// here is what only this domain can answer - what one atom of a Value means to
// Placement, what one row of Placement's own directory means, and when there
// is no list of alternatives to enumerate at all.
package store

import (
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one exact Placement route reached from a Value reference. Tag is
// the one-based dense Heap coordinate carried by the engine selection; Key is
// the owner-issued Heap root identity.
type Route struct {
	Key heap.Key
	Tag uint64
}

// authenticated fences every judgment below on the two schema authorities and
// the Value-owned candidate before any of them looks at a value. It is stated
// once because all three ask the same question first, and a judgment that
// answered about an unauthenticated transfer would answer about nothing.
func authenticated(schema placement.Schema, values *valuedomain.Schema, transfer valuedomain.StorageTransfer) bool {
	return schema.Valid() && values != nil && values.Valid() &&
		values.OwnsHeapSchema(schema.Heap()) && values.OwnsStorageTransfer(transfer)
}

// BeyondAllocations answers whether this transfer's route set has a closed
// list of alternatives at all.
//
// It does not, in two ways, and both are properties of the whole invocation
// rather than of the value alone. A Value that is Top named no alternatives.
// A Value carrying an opaque reference named some and admitted there are
// others. Either way the sound answer is every allocation the owner has, which
// only Placement's own directory can produce.
//
// A frame-local transfer is the third case and the reason the candidate is
// part of the question: it reaches no store, so it has no routes to widen TO,
// and answering from the value alone would widen a whole directory to discover
// that every row of it declines.
func BeyondAllocations(schema placement.Schema, values *valuedomain.Schema, transfer valuedomain.StorageTransfer, fact valuedomain.Value) (bool, bool) {
	if !authenticated(schema, values, transfer) {
		return false, false
	}
	// Authenticate the relation before reading Top: the extremum is
	// owner-local and a foreign Value must not widen this Heap. This is the
	// only judgment of the derivation that runs whether or not the value
	// yields an atom, so a Bottom or foreign relation is refused here or
	// nowhere.
	if !values.Equal(fact, fact) {
		return false, false
	}
	if !transfer.Persistent() {
		return false, true
	}
	if fact.IsTop() {
		return true, true
	}
	for index, count := 0, values.ValueAtomCount(fact); index < count; index++ {
		atom, atomOK := values.ValueAtomAt(fact, index)
		if !atomOK {
			return false, false
		}
		classification, classificationOK := placement.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			return false, false
		}
		if classification.Class == placement.AtomClassOpaque {
			return true, true
		}
	}
	return false, true
}

// ResolveRoute answers what one atom of a Value contributes to a transfer's
// route set.
//
// An allocation atom contributes the Placement coordinate of its Heap root,
// tagged by the one-based dense position that root occupies. Every other exact
// class - a scalar, a boot handle, a structural root - carries no local route
// and contributes nothing; that is an absence, not a refusal, which is how a
// judgment declines an item without failing the set. A frame-local transfer
// declines every atom for the same reason it widens to nothing.
//
// An opaque atom reaching here is a contradiction: the endpoint answers before
// this arm is entered, so a set still being enumerated has none. It refuses
// rather than quietly dropping an alternative the answer depends on.
func ResolveRoute(schema placement.Schema, values *valuedomain.Schema, transfer valuedomain.StorageTransfer, atom valuedomain.Atom) (Route, bool, bool) {
	if !authenticated(schema, values, transfer) {
		return Route{}, false, false
	}
	if !transfer.Persistent() {
		return Route{}, false, true
	}
	classification, classificationOK := placement.ClassifyAtom(values, atom)
	if !classificationOK || !classification.Valid() {
		return Route{}, false, false
	}
	switch classification.Class {
	case placement.AtomClassAllocation:
	case placement.AtomClassOpaque:
		return Route{}, false, false
	default:
		return Route{}, false, true
	}
	key := classification.Key
	heapSchema := schema.Heap()
	if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation {
		return Route{}, false, false
	}
	// Heap numbers its own allocation roots and Placement's directory is that
	// numbering; the round trip proves the two agree rather than assuming it.
	dense, denseOK := heapSchema.AllocationKeyIndex(key)
	canonical, canonicalOK := schema.KeyAt(dense)
	if !denseOK || dense < 0 || !canonicalOK || canonical != key {
		return Route{}, false, false
	}
	return Route{Key: key, Tag: uint64(dense) + 1}, true, true
}

// ResolveDirectoryRoute answers what one row of Placement's own coordinate
// directory contributes to a widened route set.
//
// The directory is every coordinate the owner has; only its allocation roots
// are routes, and the rest decline. It is a separate judgment from ResolveRoute
// because a directory row is a Heap key the owner already numbered, not an
// atom of a Value that has to be classified and authenticated back to one.
func ResolveDirectoryRoute(schema placement.Schema, values *valuedomain.Schema, transfer valuedomain.StorageTransfer, key heap.Key) (Route, bool, bool) {
	if !authenticated(schema, values, transfer) {
		return Route{}, false, false
	}
	if !transfer.Persistent() {
		return Route{}, false, true
	}
	dense, denseOK := schema.KeyIndex(key)
	if !denseOK {
		return Route{}, false, false
	}
	if key.Kind() != heap.RootAllocation {
		return Route{}, false, true
	}
	return Route{Key: key, Tag: uint64(dense) + 1}, true, true
}
