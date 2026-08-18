package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// address is the snapshot-relative coordinate a Locator carries. Its fields
// are unexported and no exported function accepts one, so a Locator can be
// held, copied, and compared but never minted, edited, or written down as a
// durable key. That is the whole enforcement of the rule that a Locator is an
// address rather than an identity.
type address struct {
	slot uint32
}

// Locator is a directory resolution: one column slot in one store at one
// generation. It is valid only against the snapshot that issued it, and it
// stops being valid the moment that store advances.
type Locator = identity.Locator[address]

// Snapshot is the published value. It is immutable once sealed, and every
// field is unexported, so no exported surface can write one after
// publication. Copying a Snapshot shares the published structure and copies
// no rows: a copy answers exactly what the original answers.
//
// The mutable side of the same state lives in the engine and is written only
// inside an epoch. A consumer holds snapshots and never the store.
type Snapshot struct {
	schema     identity.ContentID
	store      identity.StoreID
	generation identity.Generation
	// columns holds one erased *column[K, V] per dense slot. The slice is
	// never handed out and never appended to after Seal.
	columns []any
	// directory is the immutable ContentID to slot mapping Resolve consults.
	// It holds at most one entry per published identity and, like every
	// other published structure here, is shared with the publications
	// derived from this one rather than copied.
	directory    *trie[identity.ContentID, uint32]
	denominators Denominators
	mounts       Mounts
	queries      Queries
}

// Schema returns the identity of the sealed schema this snapshot was
// published against. A read whose axis names another schema is rejected.
func (s Snapshot) Schema() identity.ContentID { return s.schema }

// Store returns the identity of the store that published this snapshot.
func (s Snapshot) Store() identity.StoreID { return s.store }

// Generation returns the store revision this snapshot publishes. It is the
// fence every Locator issued from this snapshot is anchored to.
func (s Snapshot) Generation() identity.Generation { return s.generation }

// Published reports whether s is a sealed snapshot rather than a zero value.
func (s Snapshot) Published() bool {
	return s.schema.Available() && s.store.Available() && s.generation.Available()
}

// Columns reports how many dense column slots this snapshot publishes. It
// bounds the slot of every valid axis and every Locator.
func (s Snapshot) Columns() int { return len(s.columns) }

// Denominators returns the snapshot's sealed denominator publication.
func (s Snapshot) Denominators() Denominators { return s.denominators }

// Mounts returns the snapshot's sealed mount bindings.
func (s Snapshot) Mounts() Mounts { return s.mounts }

// Queries returns the snapshot's sealed query publication.
func (s Snapshot) Queries() Queries { return s.queries }

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
	stored, recovered := columnAt[K, V](s, ax.SchemaID, ax.Slot)
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
	stored, recovered := columnAtBuilder[K, V](b, ax.SchemaID, ax.Slot)
	if !recovered {
		var zero V
		return zero, ReadInvalid
	}
	return stored.read(key)
}

// ReadAt answers key on the column locator addresses. It validates the
// locator against this snapshot's store and generation before it validates
// bounds and column kind, so an address issued against a superseded
// generation fails closed rather than reading whatever now occupies the slot.
//
// The returned value is borrowed and transitively immutable. ReadAt allocates
// nothing.
func ReadAt[K comparable, V any](s *Snapshot, locator Locator, key K) (V, ReadStatus) {
	var zero V
	if s == nil || !locator.Valid(s.store, s.generation) {
		return zero, ReadInvalid
	}
	stored, recovered := columnAt[K, V](s, s.schema, locator.Slot.slot)
	if !recovered {
		return zero, ReadInvalid
	}
	return stored.read(key)
}

// Resolve returns the single Locator this snapshot's immutable directory
// publishes for id, anchored to this snapshot's store and generation. A
// ContentID the directory does not publish resolves to nothing; the directory
// never holds two locators for one identity. Resolve allocates nothing.
func Resolve(s *Snapshot, id identity.ContentID) (Locator, bool) {
	if s == nil || !s.Published() {
		return Locator{}, false
	}
	slot, published := trieLookup(s.directory, hashKey(identityPlan, id), id)
	if !published || uint64(slot) >= uint64(len(s.columns)) {
		return Locator{}, false
	}
	return identity.NewLocator(s.store, s.generation, address{slot: slot}), true
}

// columnAt performs the shared validation and typed recovery: schema match,
// slot bound, then the checked recovery that is the column-kind check. It
// exists so Read and ReadAt cannot drift apart on what a valid read means.
func columnAt[K comparable, V any](s *Snapshot, schema identity.ContentID, slot uint32) (*column[K, V], bool) {
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
func columnAtBuilder[K comparable, V any](b *Builder, schema identity.ContentID, slot uint32) (*column[K, V], bool) {
	if b == nil || !b.schema.Available() || !schema.Available() || schema != b.schema {
		return nil, false
	}
	if uint64(slot) >= uint64(len(b.columns)) || b.columns[slot] == nil {
		return nil, false
	}
	stored, recovered := b.columns[slot].(*column[K, V])
	return stored, recovered
}
