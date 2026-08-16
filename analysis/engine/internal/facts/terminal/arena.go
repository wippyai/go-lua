// Package terminal owns cold interning and sealed identity of typed fact
// terminals.  It deliberately knows neither factors, keys, guards, nor
// lattice operations.
package terminal

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
// starts its initial generation open for cold admission; Seal freezes it.
// Begin then derives a candidate page without copying the frozen pages.
//
// Every Arena derived from one New call is one semantic terminal owner.  It
// accepts every sealed page of that owner, including independently sealed
// sibling pages, while rejecting every candidate page until its Work seals.
// This lets one fact diagram retain terminals from convergent candidates
// without creating a diagram per transaction.
type Arena[V any] struct {
	equal       func(V, V) bool
	fingerprint func(V) uint64
	owner       *owner[V]
	page        *page[V]
	sealed      bool
}

// owner is the immutable semantic identity shared by every generation made
// from one New call.  It deliberately contains no mutable global intern table:
// interning remains candidate-local, so concurrent candidates never contend or
// make each other's provisional IDs observable.
type owner[V any] struct{ marker byte }

// page is immutable once its owning Arena or Work is sealed.  A page owns
// only its generation's additions; parent links provide structural sharing of
// its exact base generation for candidate-local exact interning.
type page[V any] struct {
	owner     *owner[V]
	parent    *page[V]
	published bool
	values    []V
	buckets   map[uint64][]ID[V]
}

// Work owns one candidate terminal page above a sealed base Arena.  It is
// single-writer construction state: candidate IDs are valid only through this
// Work until Seal publishes a derived immutable Arena.
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
	identity := &owner[V]{}
	return &Arena[V]{
		equal:       config.Equal,
		fingerprint: config.Fingerprint,
		owner:       identity,
		page:        newPage[V](identity, nil),
	}, true
}

// Admit interns value before Seal.  The returned identity is stable within
// this arena; hash collisions remain disambiguated by exact equality.
func (arena *Arena[V]) Admit(value V) (ID[V], bool) {
	if arena == nil || arena.sealed {
		return ID[V]{}, false
	}
	return admit(arena.page, arena.equal, arena.fingerprint, value)
}

// Seal closes admission.  It is idempotent and keeps exact lookup available.
func (arena *Arena[V]) Seal() bool {
	if arena == nil || arena.page == nil {
		return false
	}
	arena.sealed = true
	arena.page.published = true
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
	return &Work[V]{base: arena, page: newPage(arena.owner, arena.page), open: true}
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

// Equal reports exact semantic equality of two terminal identities from this
// sealed owner.  Candidate-local interning permits equal values to acquire
// different IDs in independently sealed Works, so ID equality alone is not a
// fact-semantic equality test.  The zero undefined terminal equals only
// itself; ordinary terminal identities must both be readable through Arena.
func (arena *Arena[V]) Equal(left, right ID[V]) bool {
	zero := ID[V]{}
	if left == zero || right == zero {
		return left == right
	}
	if arena == nil || !arena.Valid(left) || !arena.Valid(right) {
		return false
	}
	if left == right {
		return true
	}
	leftValue, leftValid := rawValue(left)
	rightValue, rightValid := rawValue(right)
	return leftValid && rightValid && arena.equal(leftValue, rightValue)
}

// Lookup returns the already sealed identity for value.  It never admits a
// value and is therefore safe at the immutable fact-plane boundary.  Equal
// values retain their one canonical terminal identity.
func (arena *Arena[V]) Lookup(value V) (ID[V], bool) {
	if arena == nil || !arena.sealed || arena.page == nil {
		return ID[V]{}, false
	}
	hash := arena.fingerprint(value)
	for candidate := arena.page; candidate != nil; candidate = candidate.parent {
		for _, id := range candidate.buckets[hash] {
			stored, valid := rawValue(id)
			if valid && arena.equal(stored, value) {
				return id, true
			}
		}
	}
	return ID[V]{}, false
}

// Every visits each value in this arena's sealed ancestry exactly once. It is
// a cold admission audit used by semantic plane construction to verify that
// every pre-admitted terminal satisfies its sparse fixed-point law.
func (arena *Arena[V]) Every(visit func(V) bool) bool {
	if arena == nil || !arena.sealed || arena.page == nil || visit == nil {
		return false
	}
	for page := arena.page; page != nil; page = page.parent {
		if !page.published {
			return false
		}
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

// Admit interns value in Work's candidate page.  Equal values already in the
// sealed base retain their old identity; otherwise an ID private to this Work
// is returned until Seal.
func (work *Work[V]) Admit(value V) (ID[V], bool) {
	if work == nil || !work.open || work.base == nil || !work.base.sealed || work.page == nil {
		return ID[V]{}, false
	}
	hash := work.base.fingerprint(value)
	for candidate := work.page; candidate != nil; candidate = candidate.parent {
		for _, id := range candidate.buckets[hash] {
			stored, valid := rawValue(id)
			if valid && work.base.equal(stored, value) {
				return id, true
			}
		}
	}
	// The exact fingerprint was already computed for the ancestry lookup.
	// Reuse it for the candidate insertion: a semantic fold may admit millions
	// of final cells, and hashing each new value twice provides no additional
	// collision evidence.
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

// Seal publishes Work's candidate page as a new immutable Arena.  Candidate
// identities become valid through every sealed Arena sharing this semantic
// owner; before this call, every Arena rejects the still-private page.
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
	next := &Arena[V]{
		equal:       work.base.equal,
		fingerprint: work.base.fingerprint,
		owner:       work.base.owner,
		page:        work.page,
		sealed:      true,
	}
	work.page.published = true
	work.open = false
	return next, true
}

func newPage[V any](identity *owner[V], parent *page[V]) *page[V] {
	return &page[V]{owner: identity, parent: parent, buckets: make(map[uint64][]ID[V])}
}

func admit[V any](target *page[V], equal func(V, V) bool, fingerprint func(V) uint64, value V) (ID[V], bool) {
	if target == nil || len(target.values) == int(^uint32(0))-1 {
		return ID[V]{}, false
	}
	hash := fingerprint(value)
	for _, id := range target.buckets[hash] {
		stored, valid := rawValue(id)
		if valid && equal(stored, value) {
			return id, true
		}
	}
	return admitHashed(target, hash, value)
}

func admitHashed[V any](target *page[V], hash uint64, value V) (ID[V], bool) {
	if target == nil || len(target.values) == int(^uint32(0))-1 {
		return ID[V]{}, false
	}
	target.values = append(target.values, value)
	id := ID[V]{page: target, slot: uint32(len(target.values))}
	target.buckets[hash] = append(target.buckets[hash], id)
	return id, true
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
