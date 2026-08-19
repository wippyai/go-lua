package snapshot

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
)

// Construction errors. They name the seal rule that rejected, so a caller
// reports which invariant it violated rather than that publication "failed".
var (
	// ErrUnavailableIdentity reports that a required identity was the zero
	// value: a snapshot is never published without a schema, a store, and a
	// generation, and a directory or denominator entry is never keyed by an
	// unavailable ContentID.
	ErrUnavailableIdentity = errors.New("snapshot: unavailable identity")
	// ErrSchemaMismatch reports that an axis belongs to another schema than
	// the one being sealed.
	ErrSchemaMismatch = errors.New("snapshot: axis belongs to another schema")
	// ErrSlotFilled reports a second write to one column slot in one
	// publication. One publication writes one authoritative column per slot.
	ErrSlotFilled = errors.New("snapshot: column slot already filled")
	// ErrSlotEmpty reports that a slot below the highest declared slot has no
	// column. Slots are dense and append-only.
	ErrSlotEmpty = errors.New("snapshot: column slot has no column")
	// ErrDuplicatePublication reports a second directory or denominator
	// entry under one ContentID. A published identity resolves to at most one
	// locator, and one denominator identity has one membership authority.
	ErrDuplicatePublication = errors.New("snapshot: identity already published")
	// ErrUnknownSlot reports a publication or an edit naming a slot no column
	// fills.
	ErrUnknownSlot = errors.New("snapshot: no column at slot")
	// ErrUnprovenMembers reports a denominator key universe offered without
	// the published denominator identity that would make it provable.
	ErrUnprovenMembers = errors.New("snapshot: denominator members without a denominator identity")
	// ErrColumnKind reports that the column or denominator named by an edit
	// was built for another key or value type. It is the construction-side
	// half of the checked recovery a read performs.
	ErrColumnKind = errors.New("snapshot: column was built for another key or value type")
	// ErrUnhashableKey reports a key type whose equality this package cannot
	// hash structurally, which is what a column keyed by an interface has.
	ErrUnhashableKey = errors.New("snapshot: column key type cannot be hashed")
	// ErrStaleGeneration reports a derived publication that does not advance
	// the store. Two snapshots of one store at one generation would make one
	// locator address two different contents.
	ErrStaleGeneration = errors.New("snapshot: derived publication does not advance the generation")
)

// Content is what a writer principal hands to a column slot: the rows it
// publishes and, when it can prove the key universe it is total over, that
// denominator's published identity and members. A Content with no
// Denominator publishes a column that can report a miss and never a proven
// absence.
//
// A denominator identity carries its members exactly once. The first column
// that names an identity declares its membership; a later column names the
// same identity with no members and is sealed against the very same set,
// which is how two columns are total over one key universe without a second
// copy of it. Offering members for an already declared identity is a second
// membership authority and is rejected.
//
// Content is consumed by copy. Neither Rows nor Members becomes the sealed
// storage, so a later write to either cannot reach a published snapshot.
type Content[K comparable, V any] struct {
	Rows        map[K]V
	Denominator identity.ContentID
	Members     []K
}

// builderCore is the construction state both publication lifecycles share:
// the dense column vector, the authorship marks that separate a second write
// from a replacement, the directory, and the denominator publication. It is
// unexported and neither lifecycle copies it, so the two builders below are
// two contracts over one implementation rather than two implementations.
type builderCore struct {
	schema     identity.ContentID
	store      identity.StoreID
	generation identity.Generation
	// columns holds one erased *column[K, V] per dense slot, inherited by
	// reference from the publication this builder derives from.
	columns []any
	// authored marks the slots this builder wrote itself, which is what
	// separates a second write to one slot from a replacement of an
	// inherited column.
	authored         []bool
	directory        *trie[identity.ContentID, uint32]
	denominators     *trie[identity.ContentID, denominatorEntry]
	denominatorCount int
}

// Builder publishes the hot store: the Link and solve state an engine
// advances generation by generation. The engine fills one inside an epoch and
// consumes it by value at Seal. Every structure it accumulates is persistent,
// so sealing shares what the publication holds instead of copying it, and a
// later builder call publishes new nodes rather than writing into nodes a
// snapshot already holds.
//
// A Builder is not safe for concurrent use; it is epoch-local by
// construction.
type Builder struct {
	builderCore
	// derivedFrom is the generation this builder was derived from, and is
	// unavailable for a builder that starts from nothing. A derived
	// publication must advance it.
	derivedFrom identity.Generation
	mounts      *trie[identity.MountID, struct{}]
	mountCount  int
	queries     *trie[identity.ContentID, struct{}]
	queryCount  int
}

// FrozenBuilder publishes the cold store: content that is produced once,
// shared by every consumer that mounts it, and never revised. It writes
// columns and addresses them exactly as a Builder does, and that is the whole
// of its contract.
//
// What it does not have is the rest. There is no derivation entry point that
// accepts a Frozen, no mount binding, and no query registration, so a cold
// publication cannot advance a generation, cannot record a Link-local
// binding, and cannot answer a runtime query. The lifecycle is enforced by
// the type rather than by a mode a caller has to check.
type FrozenBuilder struct {
	builderCore
}

// NewBuilder starts a publication of schema by store at generation. It starts
// from nothing: every column it publishes is written by it.
func NewBuilder(schema identity.ContentID, store identity.StoreID, generation identity.Generation) Builder {
	return Builder{builderCore: builderCore{schema: schema, store: store, generation: generation}}
}

// coldGeneration is the single revision a frozen store ever publishes. A cold
// store has no second revision to distinguish, so its fence is a constant.
const coldGeneration = identity.Generation(1)

// NewFrozen starts the one publication of the frozen store store. It seals a
// Frozen, which is the value a consumer mounts: immutable, shared by
// reference across every mount of it, and anchored to a generation that
// nothing can advance.
func NewFrozen(schema identity.ContentID, store identity.StoreID) FrozenBuilder {
	return FrozenBuilder{builderCore: builderCore{schema: schema, store: store, generation: coldGeneration}}
}

// NewDelta starts the publication that follows base at generation. It
// inherits base's columns, directory, denominators, mount bindings and query
// publication by reference: what the change set does not touch is the very
// same structure base holds, so an unchanged column and an unchanged
// denominator cost a pointer rather than a copy.
//
// The derived publication must advance the generation of the store that
// published base; a builder derived from an unpublished snapshot seals
// nothing.
func NewDelta(base Snapshot, generation identity.Generation) Builder {
	if !base.Published() {
		return Builder{builderCore: builderCore{generation: generation}}
	}
	inherited := make([]any, len(base.columns))
	copy(inherited, base.columns)
	return Builder{
		builderCore: builderCore{
			schema:           base.schema,
			store:            base.store,
			generation:       generation,
			columns:          inherited,
			authored:         make([]bool, len(inherited)),
			directory:        base.directory,
			denominators:     base.denominators.index,
			denominatorCount: base.denominators.count,
		},
		derivedFrom: base.generation,
		mounts:      base.mounts.bound,
		mountCount:  base.mounts.count,
		queries:     base.queries.plans,
		queryCount:  base.queries.count,
	}
}

// PutColumn seals content into the column ax names. It writes one column per
// slot per publication: a second write by this builder is rejected, and a
// write over a column inherited from the publication this builder derives
// from replaces it wholesale, detaching the replaced column from the
// denominator it read against.
func PutColumn[K comparable, V any](b *Builder, ax Axis[K, V], content Content[K, V]) error {
	if b == nil {
		return fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	return putColumn(&b.builderCore, ax, content)
}

// PutFrozenColumn seals content into the cold column ax names. It is the
// whole write surface of a frozen publication: cold content is produced once
// and published whole, so there is no row edit to make and no second
// authority a later edit could introduce.
func PutFrozenColumn[K comparable, V any](b *FrozenBuilder, ax Axis[K, V], content Content[K, V]) error {
	if b == nil {
		return fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	return putColumn(&b.builderCore, ax, content)
}

// putColumn is the column publication both lifecycles perform. It takes the
// shared construction state directly rather than through an interface,
// because an interface parameter would force every builder that reaches it to
// escape and would charge a heap allocation to each publication.
func putColumn[K comparable, V any](b *builderCore, ax Axis[K, V], content Content[K, V]) error {
	if b == nil {
		return fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	if !ax.Available() || ax.SchemaID != b.schema {
		return fmt.Errorf("%w: slot %d", ErrSchemaMismatch, ax.Slot)
	}
	plan, hashable := planFor[K]()
	if !hashable {
		return fmt.Errorf("%w: slot %d", ErrUnhashableKey, ax.Slot)
	}
	if !content.Denominator.Available() && len(content.Members) > 0 {
		return fmt.Errorf("%w: slot %d", ErrUnprovenMembers, ax.Slot)
	}
	if b.authoredAt(ax.Slot) {
		return fmt.Errorf("%w: slot %d", ErrSlotFilled, ax.Slot)
	}
	attached, entry, err := sealDenominator[K](b, plan, content.Denominator, content.Members, ax.Slot)
	if err != nil {
		return err
	}
	b.detach(ax.Slot)
	b.reserve(ax.Slot)
	sealed := &column[K, V]{plan: plan, members: attached}
	if len(content.Rows) > 0 {
		rows := make([]trieEntry[K, V], 0, len(content.Rows))
		for key, value := range content.Rows {
			rows = append(rows, trieEntry[K, V]{hash: hashKey(plan, key), key: key, value: value})
		}
		sealed.rows = trieBuild(rows, make([]trieEntry[K, V], len(rows)), 0)
	}
	b.columns[ax.Slot] = sealed
	b.authored[ax.Slot] = true
	if content.Denominator.Available() {
		b.publishDenominator(content.Denominator, entry)
	}
	return nil
}

// SetRow publishes one row into the column ax names, replacing the row that
// key held. It copies the nodes on that key's path and shares every other
// node with the column it derived from, so the cost of the edit is the change
// set rather than the column.
func SetRow[K comparable, V any](b *Builder, ax Axis[K, V], key K, value V) error {
	if b == nil {
		return fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	stored, err := editable[K, V](&b.builderCore, ax)
	if err != nil {
		return err
	}
	edited := *stored
	edited.rows, _ = trieInsert(stored.rows, 0, trieEntry[K, V]{hash: hashKey(stored.plan, key), key: key, value: value})
	b.columns[ax.Slot] = &edited
	return nil
}

// RemoveRow withdraws the row key holds in the column ax names. A key the
// column's denominator covers reads as proven absent afterwards, and any
// other key reads as a miss. Removing a row the column does not hold changes
// nothing. The edit costs the removed key's path and nothing else.
func RemoveRow[K comparable, V any](b *Builder, ax Axis[K, V], key K) error {
	if b == nil {
		return fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	stored, err := editable[K, V](&b.builderCore, ax)
	if err != nil {
		return err
	}
	edited := *stored
	edited.rows, _ = trieRemove(stored.rows, 0, hashKey(stored.plan, key), key)
	b.columns[ax.Slot] = &edited
	return nil
}

// DeclareQuery seals content as the result column of the query family named
// by family, publishes the family in the directory and registers it as
// answerable, and returns the plan a consumer opens. The three facts are
// written together because a result column that is not addressable by its
// family identity, or not registered as answerable, is not a published
// answer.
func DeclareQuery[K comparable, O any](b *Builder, family identity.ContentID, slot uint32, content Content[K, O]) (QueryPlan[K, O], error) {
	if b == nil {
		return QueryPlan[K, O]{}, fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	if !family.Available() {
		return QueryPlan[K, O]{}, fmt.Errorf("%w: query family", ErrUnavailableIdentity)
	}
	if addressed, published := trieLookup(b.directory, hashKey(identityPlan, family), family); published && addressed != slot {
		return QueryPlan[K, O]{}, fmt.Errorf("%w: %s", ErrDuplicatePublication, family)
	}
	if err := PutColumn(b, Axis[K, O]{SchemaID: b.schema, Slot: slot}, content); err != nil {
		return QueryPlan[K, O]{}, err
	}
	if err := b.Publish(family, slot); err != nil {
		return QueryPlan[K, O]{}, err
	}
	if err := b.RegisterQuery(family); err != nil {
		return QueryPlan[K, O]{}, err
	}
	return QueryPlan[K, O]{SchemaID: b.schema, Slot: slot}, nil
}

// Publish records that id resolves to slot in the sealed directory. An
// identity resolves to at most one locator, so publishing a second locator
// for one identity is rejected rather than silently replacing the first.
// Publishing the locator an identity already resolves to states the same
// fact, which is what a derived publication that reseals an addressed column
// does, and is accepted.
func (b *builderCore) Publish(id identity.ContentID, slot uint32) error {
	if !id.Available() {
		return fmt.Errorf("%w: directory entry", ErrUnavailableIdentity)
	}
	if !b.filled(slot) {
		return fmt.Errorf("%w: slot %d", ErrUnknownSlot, slot)
	}
	hash := hashKey(identityPlan, id)
	published, addressed := trieLookup(b.directory, hash, id)
	if addressed {
		if published != slot {
			return fmt.Errorf("%w: %s", ErrDuplicatePublication, id)
		}
		return nil
	}
	b.directory, _ = trieInsert(b.directory, 0, trieEntry[identity.ContentID, uint32]{
		hash: hash, key: id, value: slot,
	})
	return nil
}

// Bind records that mount participated in this publication.
func (b *Builder) Bind(mount identity.MountID) error {
	if !mount.Available() {
		return fmt.Errorf("%w: mount binding", ErrUnavailableIdentity)
	}
	var added bool
	b.mounts, added = trieInsert(b.mounts, 0, trieEntry[identity.MountID, struct{}]{
		hash: hashKey(mountPlan, mount), key: mount,
	})
	if added {
		b.mountCount++
	}
	return nil
}

// RegisterQuery records that the query family plan is answerable against this
// publication.
func (b *Builder) RegisterQuery(plan identity.ContentID) error {
	if !plan.Available() {
		return fmt.Errorf("%w: query plan", ErrUnavailableIdentity)
	}
	var added bool
	b.queries, added = trieInsert(b.queries, 0, trieEntry[identity.ContentID, struct{}]{
		hash: hashKey(identityPlan, plan), key: plan,
	})
	if added {
		b.queryCount++
	}
	return nil
}

// Seal consumes the builder by value and returns the published Snapshot. It
// shares every persistent structure it publishes, which is what makes a
// publication cost its change set; the one structure it copies is the dense
// slot vector, so a later builder call cannot reach the sealed snapshot's
// columns. It rejects a publication with a missing identity, a hole in its
// dense slot range, or a derived generation that does not advance the store.
func (b Builder) Seal() (Snapshot, error) {
	if b.derivedFrom.Available() && !b.derivedFrom.Precedes(b.generation) {
		return Snapshot{}, fmt.Errorf("%w: generation %d after %d", ErrStaleGeneration, b.generation, b.derivedFrom)
	}
	published, err := b.builderCore.seal()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		publication: published,
		mounts:      Mounts{bound: b.mounts, count: b.mountCount},
		queries:     Queries{plans: b.queries, count: b.queryCount},
	}, nil
}

// Seal consumes the frozen builder by value and returns the published Frozen.
// It applies the same identity and dense-slot rules a hot publication applies;
// what it cannot do is derive, because no entry point accepts a Frozen and
// returns a builder.
func (b FrozenBuilder) Seal() (Frozen, error) {
	published, err := b.builderCore.seal()
	if err != nil {
		return Frozen{}, err
	}
	return Frozen{publication: published}, nil
}

// seal is the publication both lifecycles perform: it validates the
// identities and the dense slot range and hands over the shared immutable
// structure. The one structure it copies is the dense slot vector, so a later
// builder call cannot reach a sealed publication's columns.
func (b builderCore) seal() (publication, error) {
	if !b.schema.Available() {
		return publication{}, fmt.Errorf("%w: schema", ErrUnavailableIdentity)
	}
	if !b.store.Available() {
		return publication{}, fmt.Errorf("%w: store", ErrUnavailableIdentity)
	}
	if !b.generation.Available() {
		return publication{}, fmt.Errorf("%w: generation", ErrUnavailableIdentity)
	}
	for slot, stored := range b.columns {
		if stored == nil {
			return publication{}, fmt.Errorf("%w: slot %d", ErrSlotEmpty, slot)
		}
	}
	sealed := publication{
		schema:       b.schema,
		store:        b.store,
		generation:   b.generation,
		columns:      make([]any, len(b.columns)),
		directory:    b.directory,
		denominators: Denominators{index: b.denominators, count: b.denominatorCount},
	}
	copy(sealed.columns, b.columns)
	return sealed, nil
}

// editable recovers the column ax names for an edit. It performs the same
// checks a read performs -- schema, slot bound, column kind -- so an edit can
// never reach a column of another schema or another key and value type.
func editable[K comparable, V any](b *builderCore, ax Axis[K, V]) (*column[K, V], error) {
	if b == nil {
		return nil, fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	if !ax.Available() || ax.SchemaID != b.schema {
		return nil, fmt.Errorf("%w: slot %d", ErrSchemaMismatch, ax.Slot)
	}
	if !b.filled(ax.Slot) {
		return nil, fmt.Errorf("%w: slot %d", ErrUnknownSlot, ax.Slot)
	}
	stored, recovered := b.columns[ax.Slot].(*column[K, V])
	if !recovered {
		return nil, fmt.Errorf("%w: slot %d", ErrColumnKind, ax.Slot)
	}
	return stored, nil
}

// sealDenominator resolves the denominator a column declares. The first
// column that names an identity seals its members; a later column names the
// identity alone and is attached to the very set the first sealed, so the
// membership is stored once and referenced twice.
func sealDenominator[K comparable](b *builderCore, plan *keyPlan, id identity.ContentID, members []K, slot uint32) (*denominator[K], denominatorEntry, error) {
	if !id.Available() {
		return nil, denominatorEntry{}, nil
	}
	published, exists := trieLookup(b.denominators, hashKey(identityPlan, id), id)
	if exists {
		if len(members) > 0 {
			return nil, denominatorEntry{}, fmt.Errorf("%w: denominator %s", ErrDuplicatePublication, id)
		}
		shared, sameKey := published.set.(*denominator[K])
		if !sameKey {
			return nil, denominatorEntry{}, fmt.Errorf("%w: denominator %s", ErrColumnKind, id)
		}
		return shared, denominatorEntry{set: published.set, size: published.size, slots: withSlot(published.slots, slot)}, nil
	}
	sealed := &denominator[K]{id: id}
	if len(members) > 0 {
		covered := make([]trieEntry[K, struct{}], 0, len(members))
		for _, member := range members {
			covered = append(covered, trieEntry[K, struct{}]{hash: hashKey(plan, member), key: member})
		}
		sealed.members = trieBuild(covered, make([]trieEntry[K, struct{}], len(covered)), 0)
	}
	return sealed, denominatorEntry{set: sealed, size: trieCount(sealed.members), slots: []uint32{slot}}, nil
}

// publishDenominator records entry under id, counting a first publication.
func (b *builderCore) publishDenominator(id identity.ContentID, entry denominatorEntry) {
	var added bool
	b.denominators, added = trieInsert(b.denominators, 0, trieEntry[identity.ContentID, denominatorEntry]{
		hash: hashKey(identityPlan, id), key: id, value: entry,
	})
	if added {
		b.denominatorCount++
	}
}

// detach withdraws slot from the denominator the column it holds reads
// against, and unpublishes a denominator that no column reads against any
// more. It runs when a publication replaces an inherited column.
func (b *builderCore) detach(slot uint32) {
	if !b.filled(slot) {
		return
	}
	held, known := b.columns[slot].(provenColumn)
	if !known {
		return
	}
	id := held.denominatorID()
	if !id.Available() {
		return
	}
	hash := hashKey(identityPlan, id)
	entry, published := trieLookup(b.denominators, hash, id)
	if !published {
		return
	}
	remaining := withoutSlot(entry.slots, slot)
	if len(remaining) == 0 {
		b.denominators, _ = trieRemove(b.denominators, 0, hash, id)
		b.denominatorCount--
		return
	}
	b.denominators, _ = trieInsert(b.denominators, 0, trieEntry[identity.ContentID, denominatorEntry]{
		hash: hash, key: id, value: denominatorEntry{set: entry.set, size: entry.size, slots: remaining},
	})
}

// provenColumn is what a sealed column answers about its denominator without
// naming its key and value types. It has no exported surface: a column is
// reachable only from a builder's own slot vector.
type provenColumn interface {
	denominatorID() identity.ContentID
}

// denominatorID returns the identity of the denominator this column is total
// over, or the unavailable identity when it publishes none.
func (c *column[K, V]) denominatorID() identity.ContentID {
	if c.members == nil {
		return identity.ContentID{}
	}
	return c.members.id
}

// withSlot returns slots including slot, in ascending order.
func withSlot(slots []uint32, slot uint32) []uint32 {
	index := 0
	for index < len(slots) && slots[index] < slot {
		index++
	}
	if index < len(slots) && slots[index] == slot {
		return slots
	}
	return inserted(slots, index, slot)
}

// withoutSlot returns slots without slot.
func withoutSlot(slots []uint32, slot uint32) []uint32 {
	for index, held := range slots {
		if held == slot {
			return excluded(slots, index)
		}
	}
	return slots
}

// reserve grows the dense slot range to cover slot. Slot assignment is
// append-only: a publication never shrinks the range it inherited.
func (b *builderCore) reserve(slot uint32) {
	if uint64(slot) < uint64(len(b.columns)) {
		return
	}
	grown := make([]any, slot+1)
	copy(grown, b.columns)
	b.columns = grown
	grownAuthorship := make([]bool, slot+1)
	copy(grownAuthorship, b.authored)
	b.authored = grownAuthorship
}

// filled reports whether slot holds a column.
func (b *builderCore) filled(slot uint32) bool {
	return uint64(slot) < uint64(len(b.columns)) && b.columns[slot] != nil
}

// authoredAt reports whether this builder wrote the column at slot itself,
// which is what a second write to one slot in one publication is.
func (b *builderCore) authoredAt(slot uint32) bool {
	return uint64(slot) < uint64(len(b.authored)) && b.authored[slot]
}
