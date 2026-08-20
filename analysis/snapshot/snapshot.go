package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// publication is the immutable state both published lifecycles share: the
// sealing schema, the store and generation that fence its addresses, the
// dense column vector, the directory, and the denominators that make an
// absence provable. It is unexported and its fields are never handed out, so
// no exported surface can write one after publication.
type publication struct {
	schema     identity.ContentID
	store      identity.StoreID
	generation identity.Generation
	// columns holds one erased *column[K, V] per dense slot. The slice is
	// never handed out and never appended to after Seal.
	columns []any
	// directory is the immutable ContentID to slot mapping OpenQuery consults.
	// It holds at most one entry per published identity and, like every
	// other published structure here, is shared with the publications
	// derived from this one rather than copied.
	directory    *trie[identity.ContentID, uint32]
	denominators Denominators
}

// Schema returns the identity of the sealed schema this publication was
// published against. A read whose axis names another schema is rejected.
func (p publication) Schema() identity.ContentID { return p.schema }

// Store returns the identity of the store that published this value.
func (p publication) Store() identity.StoreID { return p.store }

// Generation returns the store revision this value publishes.
func (p publication) Generation() identity.Generation { return p.generation }

// Published reports whether p is a sealed publication rather than a zero
// value.
func (p publication) Published() bool {
	return p.schema.Available() && p.store.Available() && p.generation.Available()
}

// Columns reports how many dense column slots this value publishes. It
// bounds the slot of every valid axis.
func (p publication) Columns() int { return len(p.columns) }

// Denominators returns the sealed denominator publication.
func (p publication) Denominators() Denominators { return p.denominators }

// Snapshot is the published hot value: one store at one generation, the state
// an engine advances. Copying a Snapshot shares the published structure and
// copies no rows: a copy answers exactly what the original answers.
//
// The mutable side of the same state lives in the engine and is written only
// inside an epoch. A consumer holds snapshots and never the store.
type Snapshot struct {
	publication
	queries Queries
}

// Queries returns the snapshot's sealed query publication.
func (s Snapshot) Queries() Queries { return s.queries }

// Frozen is the published cold value: content produced once, addressed by its
// own store, and never revised. It is what a consumer mounts.
//
// Frozen is a distinct type rather than a mode of Snapshot because the
// difference is what the value admits, not what it reports. There is no
// derivation that accepts a Frozen, so its generation is final by
// construction; and it carries no query publication, because a value shared
// unchanged across every mount of it cannot own facts that belong to one
// solve. Sharing a Frozen by value shares the published structure and copies
// no rows.
type Frozen struct {
	publication
}

// Read answers key on the column ax names. It validates that the axis belongs
// to this snapshot's schema, that the slot is in bounds, and that the sealed
// column was built for exactly K and V, and only then recovers the typed
// column. Every one of those checks fails closed to ReadInvalid, so a
// mismatched read can never be mistaken for a miss and can never enter a
// reader's dependency set as one.
//
// The returned value is borrowed and transitively immutable. Read allocates
// nothing.
func Read[K comparable, V any](s *Snapshot, ax Axis[K, V], key K) (V, ReadStatus) {
	if s == nil {
		var zero V
		return zero, ReadInvalid
	}
	stored, recovered := columnAt[K, V](&s.publication, ax.SchemaID, ax.Slot)
	if !recovered {
		var zero V
		return zero, ReadInvalid
	}
	return stored.read(key)
}

// ReadOverlay answers key on the current column the builder ax names. It is
// the mutable-publication counterpart to Read: an inherited column and any
// persistent edits already applied to it are visible immediately, while the
// same schema, slot, and column-kind checks fail closed to ReadInvalid.
//
// The returned value is borrowed and transitively immutable. ReadOverlay
// allocates nothing.
func ReadOverlay[K comparable, V any](b *Builder, ax Axis[K, V], key K) (V, ReadStatus) {
	if b == nil {
		var zero V
		return zero, ReadInvalid
	}
	stored, recovered := columnAtBuilder[K, V](&b.builderCore, ax.SchemaID, ax.Slot)
	if !recovered {
		var zero V
		return zero, ReadInvalid
	}
	return stored.read(key)
}

// ReadFrozen answers key on the cold column ax names. It performs exactly the
// checks Read performs and returns the same outcomes; the separate entry
// point exists because a Frozen is a different published value, not because
// a cold read means anything different.
//
// The returned value is borrowed and transitively immutable. ReadFrozen
// allocates nothing.
func ReadFrozen[K comparable, V any](f *Frozen, ax Axis[K, V], key K) (V, ReadStatus) {
	if f == nil {
		var zero V
		return zero, ReadInvalid
	}
	stored, recovered := columnAt[K, V](&f.publication, ax.SchemaID, ax.Slot)
	if !recovered {
		var zero V
		return zero, ReadInvalid
	}
	return stored.read(key)
}

// columnAt performs the shared validation and typed recovery: schema match,
// slot bound, then the checked recovery that is the column-kind check.
func columnAt[K comparable, V any](s *publication, schema identity.ContentID, slot uint32) (*column[K, V], bool) {
	if s == nil || !s.Published() || !schema.Available() || schema != s.schema {
		return nil, false
	}
	if uint64(slot) >= uint64(len(s.columns)) {
		return nil, false
	}
	stored, recovered := s.columns[slot].(*column[K, V])
	return stored, recovered
}

// columnAtBuilder performs the builder-side validation and typed recovery for
// ReadOverlay. A builder can be read before Seal, so unlike columnAt it does
// not require store and generation identities that are only needed to publish
// a Snapshot.
func columnAtBuilder[K comparable, V any](b *builderCore, schema identity.ContentID, slot uint32) (*column[K, V], bool) {
	if b == nil || !b.schema.Available() || !schema.Available() || schema != b.schema {
		return nil, false
	}
	if uint64(slot) >= uint64(len(b.columns)) || b.columns[slot] == nil {
		return nil, false
	}
	stored, recovered := b.columns[slot].(*column[K, V])
	return stored, recovered
}
