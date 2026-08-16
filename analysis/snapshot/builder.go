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
	// ErrSlotFilled reports a second write to one column slot. One slot has
	// one authoritative column.
	ErrSlotFilled = errors.New("snapshot: column slot already filled")
	// ErrSlotEmpty reports that a slot below the highest declared slot has no
	// column. Slots are dense and append-only.
	ErrSlotEmpty = errors.New("snapshot: column slot has no column")
	// ErrDuplicatePublication reports a second directory or denominator
	// entry under one ContentID. A published identity resolves to at most one
	// locator.
	ErrDuplicatePublication = errors.New("snapshot: identity already published")
	// ErrUnknownSlot reports a publication naming a slot no column fills.
	ErrUnknownSlot = errors.New("snapshot: no column at slot")
	// ErrUnprovenMembers reports a denominator key universe offered without
	// the published denominator identity that would make it provable.
	ErrUnprovenMembers = errors.New("snapshot: denominator members without a denominator identity")
)

// Content is what a writer principal hands to a column slot: the rows it
// publishes and, when it can prove the key universe it is total over, that
// denominator's published identity and members. A Content with no
// Denominator publishes a column that can report a miss and never a proven
// absence.
//
// Content is consumed by copy. Neither Rows nor Members becomes the sealed
// storage, so a later write to either cannot reach a published snapshot.
type Content[K comparable, V any] struct {
	Rows        map[K]V
	Denominator identity.ContentID
	Members     []K
}

// Builder is the only construction surface. The engine fills one inside an
// epoch and consumes it by value at Seal. A slot is filled exactly once and
// is never reopened, so no builder call can write into a column that a
// Snapshot has already published, and no exported surface anywhere in the
// package can name a column at all.
//
// A Builder is not safe for concurrent use; it is epoch-local by
// construction.
type Builder struct {
	schema       identity.ContentID
	store        identity.StoreID
	generation   identity.Generation
	columns      []any
	directory    map[identity.ContentID]uint32
	denominators map[identity.ContentID]uint32
	mounts       map[identity.MountID]struct{}
	queries      map[identity.ContentID]struct{}
}

// NewBuilder starts a publication of schema by store at generation.
func NewBuilder(schema identity.ContentID, store identity.StoreID, generation identity.Generation) Builder {
	return Builder{
		schema:       schema,
		store:        store,
		generation:   generation,
		directory:    make(map[identity.ContentID]uint32),
		denominators: make(map[identity.ContentID]uint32),
		mounts:       make(map[identity.MountID]struct{}),
		queries:      make(map[identity.ContentID]struct{}),
	}
}

// PutColumn seals content into the column ax names. It copies the rows and
// the denominator members, and it fills the slot once: a second column at one
// slot is rejected, which is what makes a filled column unreachable from the
// builder that filled it.
func PutColumn[K comparable, V any](b *Builder, ax Axis[K, V], content Content[K, V]) error {
	if b == nil {
		return fmt.Errorf("%w: builder", ErrUnavailableIdentity)
	}
	if !ax.Available() || ax.SchemaID != b.schema {
		return fmt.Errorf("%w: slot %d", ErrSchemaMismatch, ax.Slot)
	}
	if !content.Denominator.Available() && len(content.Members) > 0 {
		return fmt.Errorf("%w: slot %d", ErrUnprovenMembers, ax.Slot)
	}
	if content.Denominator.Available() {
		if _, published := b.denominators[content.Denominator]; published {
			return fmt.Errorf("%w: denominator %s", ErrDuplicatePublication, content.Denominator)
		}
	}
	if err := b.reserve(ax.Slot); err != nil {
		return err
	}
	sealed := &column[K, V]{rows: make(map[K]V, len(content.Rows))}
	for key, value := range content.Rows {
		sealed.rows[key] = value
	}
	if content.Denominator.Available() {
		sealed.members = make(map[K]struct{}, len(content.Members))
		for _, member := range content.Members {
			sealed.members[member] = struct{}{}
		}
		b.denominators[content.Denominator] = ax.Slot
	}
	b.columns[ax.Slot] = sealed
	return nil
}

// Publish records that id resolves to slot in the sealed directory. An
// identity resolves to at most one locator, so a second publication of one
// identity is rejected rather than silently replacing the first.
func (b *Builder) Publish(id identity.ContentID, slot uint32) error {
	if !id.Available() {
		return fmt.Errorf("%w: directory entry", ErrUnavailableIdentity)
	}
	if !b.filled(slot) {
		return fmt.Errorf("%w: slot %d", ErrUnknownSlot, slot)
	}
	if _, published := b.directory[id]; published {
		return fmt.Errorf("%w: %s", ErrDuplicatePublication, id)
	}
	b.directory[id] = slot
	return nil
}

// Bind records that mount participated in this publication.
func (b *Builder) Bind(mount identity.MountID) error {
	if !mount.Available() {
		return fmt.Errorf("%w: mount binding", ErrUnavailableIdentity)
	}
	b.mounts[mount] = struct{}{}
	return nil
}

// RegisterQuery records that plan is answerable against this publication.
func (b *Builder) RegisterQuery(plan identity.ContentID) error {
	if !plan.Available() {
		return fmt.Errorf("%w: query plan", ErrUnavailableIdentity)
	}
	b.queries[plan] = struct{}{}
	return nil
}

// Seal consumes the builder by value and returns the published Snapshot. It
// copies every container it publishes, so no later builder call can extend
// what the Snapshot holds, and it rejects a publication with a missing
// identity or a hole in its dense slot range.
func (b Builder) Seal() (Snapshot, error) {
	if !b.schema.Available() {
		return Snapshot{}, fmt.Errorf("%w: schema", ErrUnavailableIdentity)
	}
	if !b.store.Available() {
		return Snapshot{}, fmt.Errorf("%w: store", ErrUnavailableIdentity)
	}
	if !b.generation.Available() {
		return Snapshot{}, fmt.Errorf("%w: generation", ErrUnavailableIdentity)
	}
	for slot, stored := range b.columns {
		if stored == nil {
			return Snapshot{}, fmt.Errorf("%w: slot %d", ErrSlotEmpty, slot)
		}
	}
	sealed := Snapshot{
		schema:     b.schema,
		store:      b.store,
		generation: b.generation,
		columns:    make([]any, len(b.columns)),
		directory:  make(map[identity.ContentID]uint32, len(b.directory)),
		denominators: Denominators{
			slots: make(map[identity.ContentID]uint32, len(b.denominators)),
		},
		mounts:  Mounts{bound: make(map[identity.MountID]struct{}, len(b.mounts))},
		queries: Queries{plans: make(map[identity.ContentID]struct{}, len(b.queries))},
	}
	copy(sealed.columns, b.columns)
	for id, slot := range b.directory {
		sealed.directory[id] = slot
	}
	for id, slot := range b.denominators {
		sealed.denominators.slots[id] = slot
	}
	for mount := range b.mounts {
		sealed.mounts.bound[mount] = struct{}{}
	}
	for plan := range b.queries {
		sealed.queries.plans[plan] = struct{}{}
	}
	return sealed, nil
}

// reserve grows the dense slot range to cover slot and rejects a second
// column at one slot. Slot assignment is append-only and a filled slot is
// never reopened.
func (b *Builder) reserve(slot uint32) error {
	if uint64(slot) >= uint64(len(b.columns)) {
		grown := make([]any, slot+1)
		copy(grown, b.columns)
		b.columns = grown
		return nil
	}
	if b.columns[slot] != nil {
		return fmt.Errorf("%w: slot %d", ErrSlotFilled, slot)
	}
	return nil
}

// filled reports whether slot holds a column.
func (b *Builder) filled(slot uint32) bool {
	return uint64(slot) < uint64(len(b.columns)) && b.columns[slot] != nil
}
