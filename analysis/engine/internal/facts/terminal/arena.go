// Package terminal owns cold interning and sealed identity of typed fact
// terminals.  It deliberately knows neither factors, keys, guards, nor
// lattice operations.
package terminal

import "sync"

// ID is a typed immutable-page terminal identity.  Its semantic owner and
// page are private, so a same-numbered terminal from another Arena cannot
// enter a fact diagram.  The zero value denotes no terminal.
type ID[V any] struct {
	page *page[V]
	slot uint32
}

// Config supplies exact semantic equality and its collision-tolerant index.
// Equal values must produce the same Fingerprint; matching fingerprints never
// prove equality by themselves.
type Config[V any] struct {
	Equal       func(V, V) bool
	Fingerprint func(V) uint64
}

// Arena is one immutable generation of a typed terminal universe.  New
// starts its initial generation open for cold admission; Seal freezes it and
// promotes its values into the owner's sealed intern generation.  Begin then
// derives a candidate page without copying the frozen pages.
//
// Every Arena derived from one New call is one semantic terminal owner, and
// that owner holds one intern generation over every page it has published.
// Equal values therefore share one terminal identity across sibling
// candidates and across seals, so sealed identity is the exact equality
// answer and no consumer needs a second representative table.
type Arena[V any] struct {
	equal       func(V, V) bool
	fingerprint func(V) uint64
	owner       *owner[V]
	page        *page[V]
	sealed      bool
}

// owner is the immutable semantic identity shared by every generation made
// from one New call.  It holds the sealed intern generation: a page joins it
// at publication, never before, so concurrent candidates remain mutually
// invisible while every published value keeps exactly one canonical identity.
type owner[V any] struct {
	mutex   sync.RWMutex
	buckets map[uint64][]ID[V]
	pages   []*page[V]
}

// page is immutable once its owning Arena or Work is sealed.  buckets is the
// candidate-local exact index that resolves repeats before publication; it is
// released at promotion, when the owner index takes over.  hashes retains the
// exact fingerprint each slot was admitted under so promotion never rehashes a
// value.  forward names the canonical identity of a slot whose value an
// independent page published first, so a duplicate admitted under candidate
// isolation still resolves to one terminal.
type page[V any] struct {
	owner     *owner[V]
	published bool
	values    []V
	hashes    []uint64
	forward   []ID[V]
	buckets   map[uint64][]ID[V]
}

// Work owns one candidate terminal page above a sealed base Arena.  It is
// single-writer construction state: candidate IDs are valid only through this
// Work until Seal promotes them, and Discard drops them entirely.
type Work[V any] struct {
	base *Arena[V]
	page *page[V]
	open bool
}

// New creates an open terminal arena.
func New[V any](config Config[V]) (*Arena[V], bool) {
	if config.Equal == nil || config.Fingerprint == nil {
		return nil, false
	}
	identity := &owner[V]{buckets: make(map[uint64][]ID[V])}
	return &Arena[V]{
		equal:       config.Equal,
		fingerprint: config.Fingerprint,
		owner:       identity,
		page:        newPage(identity),
	}, true
}

// Admit interns value before Seal.  The returned identity is stable within
// this arena; hash collisions remain disambiguated by exact equality.
func (arena *Arena[V]) Admit(value V) (ID[V], bool) {
	if arena == nil || arena.sealed || arena.page == nil {
		return ID[V]{}, false
	}
	hash := arena.fingerprint(value)
	if id, found := arena.page.local(hash, arena.equal, value); found {
		return id, true
	}
	return admitHashed(arena.page, hash, value)
}

// Seal closes admission and promotes this generation's values into the owner's
// sealed intern generation.  It is idempotent and keeps exact lookup available.
func (arena *Arena[V]) Seal() bool {
	if arena == nil || arena.page == nil {
		return false
	}
	if arena.sealed {
		return true
	}
	if !arena.owner.promote(arena.page, arena.equal) {
		return false
	}
	arena.sealed = true
	return true
}

// Sealed reports whether the arena may back immutable fact roots.
func (arena *Arena[V]) Sealed() bool { return arena != nil && arena.sealed }

// Begin opens one candidate page over this sealed generation.  It never
// mutates the base Arena, and a nil result means the base is not immutable.
func (arena *Arena[V]) Begin() *Work[V] {
	if arena == nil || !arena.sealed || arena.owner == nil || arena.page == nil {
		return nil
	}
	return &Work[V]{base: arena, page: newPage(arena.owner), open: true}
}

// BeginInto opens the same candidate page over caller-owned Work storage. A
// candidate page that published nothing is not part of any sealed identity, so
// the next candidate reuses it and a warm write transaction admits terminals
// without allocating a page at all. A page the owner has published is
// immutable and is never reused.
func (arena *Arena[V]) BeginInto(work *Work[V]) bool {
	if arena == nil || work == nil || work.open || !arena.sealed || arena.owner == nil || arena.page == nil {
		return false
	}
	if work.page == nil || work.page.owner != arena.owner || work.page.published {
		work.page = newPage(arena.owner)
	} else {
		work.page.reuse()
	}
	work.base, work.open = arena, true
	return true
}

// Valid reports whether id names a published page from this Arena's one
// immutable semantic owner.  It intentionally accepts independently sealed
// sibling generations, but never a still-open Work page.
func (arena *Arena[V]) Valid(id ID[V]) bool {
	return arena != nil && arena.sealed && arena.owner != nil && validPageID(id) && id.page.owner == arena.owner && id.page.published
}

// Value returns an exact sealed terminal value.
func (arena *Arena[V]) Value(id ID[V]) (V, bool) {
	if !arena.Valid(id) {
		var zero V
		return zero, false
	}
	return id.page.values[int(id.slot)-1], true
}

// Canonical returns the one identity this owner published for id's semantic
// value.  Equal published terminals share it, so an identity comparison is the
// exact sealed equality test.  An identity this Arena cannot read resolves to
// the zero terminal.
func (arena *Arena[V]) Canonical(id ID[V]) ID[V] {
	if !arena.Valid(id) {
		return ID[V]{}
	}
	return canonical(id)
}

// Equal reports exact semantic equality of two terminal identities from this
// sealed owner.  Publication canonicalizes equal values onto one identity, so
// this is a constant-time identity answer rather than a value comparison. The
// zero undefined terminal equals only itself; ordinary terminal identities
// must both be readable through Arena.
func (arena *Arena[V]) Equal(left, right ID[V]) bool {
	zero := ID[V]{}
	if left == zero || right == zero {
		return left == right
	}
	if arena == nil || !arena.Valid(left) || !arena.Valid(right) {
		return false
	}
	return canonical(left) == canonical(right)
}

// Lookup returns the already sealed identity for value.  It never admits a
// value and is therefore safe at the immutable fact-plane boundary.  Equal
// values retain their one canonical terminal identity.
func (arena *Arena[V]) Lookup(value V) (ID[V], bool) {
	if arena == nil || !arena.sealed || arena.owner == nil {
		return ID[V]{}, false
	}
	return arena.owner.lookup(arena.fingerprint(value), arena.equal, value)
}

// Every visits each value of this owner's sealed universe exactly once. It is
// a cold admission audit used by semantic plane construction to verify that
// every pre-admitted terminal satisfies its sparse fixed-point law.
func (arena *Arena[V]) Every(visit func(V) bool) bool {
	if arena == nil || !arena.sealed || arena.owner == nil || visit == nil {
		return false
	}
	for _, page := range arena.owner.published() {
		for _, value := range page.values {
			if !visit(value) {
				return false
			}
		}
	}
	return true
}

// Base returns Work's sealed immutable predecessor.  It is useful to
// consumers that must prove a candidate root and terminal work share one
// generation before publication.
func (work *Work[V]) Base() *Arena[V] {
	if work == nil {
		return nil
	}
	return work.base
}

// Admit interns value for this candidate page.  A value the owner already
// published keeps that canonical identity; otherwise an ID private to this
// Work is returned until Seal promotes it.
func (work *Work[V]) Admit(value V) (ID[V], bool) {
	if work == nil || !work.open || work.base == nil || !work.base.sealed || work.page == nil {
		return ID[V]{}, false
	}
	// The exact fingerprint is computed once and reused for the owner probe,
	// the candidate probe, and the candidate insertion: a semantic fold may
	// admit millions of final cells, and hashing a value again provides no
	// additional collision evidence.
	hash := work.base.fingerprint(value)
	if id, found := work.base.owner.lookup(hash, work.base.equal, value); found {
		return id, true
	}
	if id, found := work.page.local(hash, work.base.equal, value); found {
		return id, true
	}
	return admitHashed(work.page, hash, value)
}

// Valid accepts published IDs from Work's sealed base owner (including sealed
// sibling pages) and IDs created in this still-open candidate page. It rejects
// IDs from a sibling candidate page until that page is sealed.
func (work *Work[V]) Valid(id ID[V]) bool {
	return work != nil && work.open && validPageID(id) && (work.base.Valid(id) || id.page == work.page)
}

// Value resolves an exact terminal value from Work's base or candidate page.
func (work *Work[V]) Value(id ID[V]) (V, bool) {
	if !work.Valid(id) {
		var zero V
		return zero, false
	}
	return id.page.values[int(id.slot)-1], true
}

// Seal promotes Work's candidate page into the owner's sealed intern
// generation and publishes it as a new immutable Arena.  Candidate identities
// become valid through every sealed Arena sharing this semantic owner; before
// this call, every Arena rejects the still-private page.
func (work *Work[V]) Seal() (*Arena[V], bool) {
	if work == nil || !work.open || work.base == nil || !work.base.sealed || work.page == nil {
		return nil, false
	}
	// A fact patch with no surviving dynamic terminals must not create an
	// empty immutable generation.  The sealed predecessor is already the exact
	// terminal authority for its published FDD root.
	if len(work.page.values) == 0 {
		work.open = false
		return work.base, true
	}
	if !work.base.owner.promote(work.page, work.base.equal) {
		return nil, false
	}
	next := &Arena[V]{
		equal:       work.base.equal,
		fingerprint: work.base.fingerprint,
		owner:       work.base.owner,
		page:        work.page,
		sealed:      true,
	}
	work.open = false
	return next, true
}

// Discard abandons this candidate page.  Its terminals never reach the owner's
// sealed intern generation and never become readable identities.
func (work *Work[V]) Discard() {
	if work == nil {
		return
	}
	work.open = false
	if work.page != nil && work.page.published {
		work.page = nil
	}
}

// promote publishes target into the owner's one sealed intern generation.
// A value an independent page published first keeps that earlier identity, and
// target's own slot forwards to it, so equal published terminals never present
// two identities.  Publication happens under the same lock, which is the
// ordering edge that makes the promoted page readable.
func (identity *owner[V]) promote(target *page[V], equal func(V, V) bool) bool {
	if identity == nil || target == nil || len(target.hashes) != len(target.values) {
		return false
	}
	identity.mutex.Lock()
	defer identity.mutex.Unlock()
	for index, value := range target.values {
		hash := target.hashes[index]
		if existing, found := lookupBucket(identity.buckets[hash], equal, value); found {
			if target.forward == nil {
				target.forward = make([]ID[V], len(target.values))
			}
			target.forward[index] = existing
			continue
		}
		identity.buckets[hash] = append(identity.buckets[hash], ID[V]{page: target, slot: uint32(index) + 1})
	}
	identity.pages = append(identity.pages, target)
	// The candidate index and its admission fingerprints have been superseded
	// by the owner generation.
	target.buckets = nil
	target.hashes = nil
	target.published = true
	return true
}

func (identity *owner[V]) lookup(hash uint64, equal func(V, V) bool, value V) (ID[V], bool) {
	if identity == nil {
		return ID[V]{}, false
	}
	identity.mutex.RLock()
	defer identity.mutex.RUnlock()
	return lookupBucket(identity.buckets[hash], equal, value)
}

func (identity *owner[V]) published() []*page[V] {
	if identity == nil {
		return nil
	}
	identity.mutex.RLock()
	defer identity.mutex.RUnlock()
	return identity.pages[:len(identity.pages):len(identity.pages)]
}

func lookupBucket[V any](bucket []ID[V], equal func(V, V) bool, value V) (ID[V], bool) {
	for _, id := range bucket {
		stored, valid := rawValue(id)
		if valid && equal(stored, value) {
			return id, true
		}
	}
	return ID[V]{}, false
}

func newPage[V any](identity *owner[V]) *page[V] {
	return &page[V]{owner: identity}
}

// reuse returns an unpublished candidate page to its empty state while keeping
// the storage it has already grown. A published page is immutable and refuses.
func (target *page[V]) reuse() {
	if target == nil || target.published {
		return
	}
	clear(target.values)
	clear(target.forward)
	target.values = target.values[:0]
	target.hashes = target.hashes[:0]
	target.forward = target.forward[:0]
	clear(target.buckets)
}

// local resolves a repeat within this still-open candidate page.  Every entry
// it holds already missed the owner generation at its own admission.
func (target *page[V]) local(hash uint64, equal func(V, V) bool, value V) (ID[V], bool) {
	if target == nil {
		return ID[V]{}, false
	}
	return lookupBucket(target.buckets[hash], equal, value)
}

func admitHashed[V any](target *page[V], hash uint64, value V) (ID[V], bool) {
	if target == nil || target.published || len(target.values) == int(^uint32(0))-1 {
		return ID[V]{}, false
	}
	if target.buckets == nil {
		// A transaction that only reuses already interned values never needs a
		// candidate index, and a point fold performs many such transactions.
		target.buckets = make(map[uint64][]ID[V])
	}
	target.values = append(target.values, value)
	target.hashes = append(target.hashes, hash)
	id := ID[V]{page: target, slot: uint32(len(target.values))}
	target.buckets[hash] = append(target.buckets[hash], id)
	return id, true
}

// canonical resolves a published slot onto the one identity its owner holds
// for that semantic value.  Forwarding is one hop by construction: a forward
// target is always an identity the owner generation itself indexes.
func canonical[V any](id ID[V]) ID[V] {
	if int(id.slot) <= len(id.page.forward) {
		if target := id.page.forward[id.slot-1]; target.page != nil {
			return target
		}
	}
	return id
}

func validPageID[V any](id ID[V]) bool {
	return id.page != nil && id.slot != 0 && int(id.slot) <= len(id.page.values)
}

func rawValue[V any](id ID[V]) (V, bool) {
	if !validPageID(id) {
		var zero V
		return zero, false
	}
	return id.page.values[int(id.slot)-1], true
}
