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
	// directory is the immutable ContentID to slot mapping Resolve consults.
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

// Generation returns the store revision this value publishes. It is the
// fence every Locator issued from it is anchored to.
func (p publication) Generation() identity.Generation { return p.generation }

// Published reports whether p is a sealed publication rather than a zero
// value.
func (p publication) Published() bool {
	return p.schema.Available() && p.store.Available() && p.generation.Available()
}

// Columns reports how many dense column slots this value publishes. It
// bounds the slot of every valid axis and every Locator.
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
	stored, recovered := columnAt[K, V](&s.publication, s.schema, locator.Slot.slot)
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
	if s == nil {
		return Locator{}, false
	}
	return s.publication.resolve(id)
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

// ResolveFrozen returns the single Locator the cold directory publishes for
// id, anchored to the frozen store and its final generation. Because that
// generation never advances, a Locator issued here stays valid for as long as
// the Frozen value exists. ResolveFrozen allocates nothing.
func ResolveFrozen(f *Frozen, id identity.ContentID) (Locator, bool) {
	if f == nil {
		return Locator{}, false
	}
	return f.publication.resolve(id)
}

// ReadFrozenAt answers key on the cold column locator addresses.
//
// The returned value is borrowed and transitively immutable. ReadFrozenAt
// allocates nothing.
func ReadFrozenAt[K comparable, V any](f *Frozen, locator Locator, key K) (V, ReadStatus) {
	var zero V
	if f == nil || !locator.Valid(f.store, f.generation) {
		return zero, ReadInvalid
	}
	stored, recovered := columnAt[K, V](&f.publication, f.schema, locator.Slot.slot)
	if !recovered {
		return zero, ReadInvalid
	}
	return stored.read(key)
}

// resolve is the directory lookup both published lifecycles perform.
func (p *publication) resolve(id identity.ContentID) (Locator, bool) {
	if p == nil || !p.Published() {
		return Locator{}, false
	}
	slot, published := trieLookup(p.directory, hashKey(identityPlan, id), id)
	if !published || uint64(slot) >= uint64(len(p.columns)) {
		return Locator{}, false
	}
	return identity.NewLocator(p.store, p.generation, address{slot: slot}), true
}

// columnAt performs the shared validation and typed recovery: schema match,
// slot bound, then the checked recovery that is the column-kind check. It
// exists so Read and ReadAt cannot drift apart on what a valid read means.
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
