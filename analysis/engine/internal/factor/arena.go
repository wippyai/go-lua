// Package factor implements the Solver's typed persistent sparse-factor
// storage.  An Arena is an immutable definition; all construction happens in
// a candidate-private Work and only sealed Roots may cross that boundary.
package factor

import "github.com/wippyai/go-lua/analysis/lattice"

const (
	radixBits  uint8 = 5
	radixMask        = (1 << radixBits) - 1
	hashLevels uint8 = (64 + radixBits - 1) / radixBits
)

// ref is the sole physical identity used by Factor storage. It is a direct
// GC-traced page reference plus a slot in that page. A zero ref denotes no
// cell; it is used for absent HAMT children only.
//
// Its fields deliberately remain private.  The one typed recovery from its
// header lives in storage.go and validates Arena ownership before unsafe is
// involved.
type ref struct {
	header *pageHeader
	slot   int
}

func (ref ref) zero() bool { return ref.header == nil }

// Handle is an opaque physical Factor-root capability. It deliberately carries
// no K/V parameters so one private Solver vector can retain roots from
// heterogeneous typed Factors without []any or a value-level decoding path.
//
// A Handle is not independently readable: Arena and Work validate its owner
// before converting it back to a typed Root. Its representation remains the
// existing compact page/slot ref; no second root arena or indirection exists.
type Handle struct{ ref ref }

// Root is an immutable typed Factor snapshot capability. Generic typing is
// necessary: it prevents a root from one K/V store being passed to another
// without a deliberate internal forgery, which Arena validation rejects.
type Root[K ~uint64, V any] struct{ ref ref }

// KeyRange is one sealed, finite direct-key universe. End denotes [0, End).
// The zero value is the valid empty range.
//
// This is cardinality mathematics, not a storage capacity, iteration bound,
// admission quota, widening fallback, or semantic approximation. Dynamic or
// unbounded domain support belongs in the Factor value, where the domain owns
// its canonical partition and convergence law.
type KeyRange struct {
	End uint64
}

func (KeyRange) valid() bool { return true }

func (keys KeyRange) contains(key uint64) bool {
	return key < keys.End
}

// Config supplies one Factor's immutable semantic, direct-key universe, and
// termination contracts. Default is explicit because it need not be a domain
// Bottom. Narrow is optional, but whenever it is supplied its independent
// well-founded descent witness is mandatory; the Solver may enter a narrowing
// phase only when every Factor in its correlated tuple has this contract.
//
// V is a persistent immutable snapshot. Equal, Same, Join, and Widen
// must be pure and deterministic, must never mutate their inputs, and must
// return immutable snapshots. Factor structurally shares V values and may
// call these operations repeatedly while constructing a candidate; mutation
// would invalidate both persistence and formal fixed-point reasoning.
type Config[K ~uint64, V any] struct {
	KeyRange KeyRange
	Lattice  lattice.Lattice[V]
	Default  V
	// Fingerprint is a deterministic, equality-consistent summary of one
	// semantic value: Equal values must produce the same result. Collisions
	// are permitted and never decide Factor equality; they only place roots
	// into a candidate bucket before exact Equal validates them.
	Fingerprint func(V) uint64
	// WidenRank proves every strict pointwise Widen transition descends. It may
	// be absent only for a Factor used solely in acyclic compiled equations;
	// the Solver rejects a cyclic tuple containing an unranked Factor before
	// evaluation. Factor is where both the key and the value are available to
	// validate a supplied proof at the actual logical coordinate.
	WidenRank Measure[K, V]
	// NarrowRank proves every strict pointwise Narrow transition descends. It
	// is absent exactly when Lattice.Narrow is absent.
	NarrowRank Measure[K, V]
}

// Arena is a pure typed Factor definition.  It retains no mutable page list,
// node table, key interner, cache, or construction history.
// The sealed empty root is the sole storage it owns.
type Arena[K ~uint64, V any] struct {
	owner        *arenaOwner
	keyRange     KeyRange
	values       lattice.Lattice[V]
	defaultValue V
	fingerprint  func(V) uint64
	widenRank    Measure[K, V]
	narrowRank   Measure[K, V]
	empty        Root[K, V]
}

// arenaOwner is a non-zero-sized, allocation-identity token. Its address is
// the private authority carried by every page of one Arena. A zero-sized Go
// type would be wrong here: separate allocations may share its address.
// The semantic requirement is language-neutral: an Arena owns one unique,
// unforgeable page authority token (a Rust implementation can use an
// allocation-backed private token in exactly the same way).
type arenaOwner struct{ marker byte }

// New validates the semantic storage contract and creates an immutable Arena
// definition with one sealed empty-root page.
func New[K ~uint64, V any](config Config[K, V]) (*Arena[K, V], bool) {
	if !config.KeyRange.valid() ||
		config.Lattice.Bottom == nil || config.Lattice.Top == nil ||
		config.Lattice.Equal == nil || config.Lattice.LessOrEq == nil ||
		config.Lattice.Join == nil || config.Lattice.Widen == nil ||
		config.Fingerprint == nil ||
		!config.WidenRank.valid() && !config.WidenRank.absent() ||
		config.Lattice.Narrow != nil && !config.NarrowRank.valid() ||
		config.Lattice.Narrow == nil && (config.NarrowRank.Width != 0 || config.NarrowRank.At != nil) {
		return nil, false
	}
	defaultJoin := config.Lattice.Join(config.Default, config.Default)
	defaultWiden := config.Lattice.Widen(config.Default, config.Default)
	if !sameWith(config.Lattice, defaultJoin, config.Default) ||
		!sameWith(config.Lattice, defaultWiden, config.Default) ||
		!config.Lattice.LessOrEq(config.Default, defaultWiden) {
		return nil, false
	}
	if !config.Lattice.LessOrEq(config.Default, defaultJoin) {
		return nil, false
	}
	if config.Lattice.Narrow != nil {
		defaultNarrow := config.Lattice.Narrow(config.Default, config.Default)
		if !sameWith(config.Lattice, defaultNarrow, config.Default) ||
			!config.Lattice.LessOrEq(config.Default, defaultNarrow) ||
			!config.Lattice.LessOrEq(defaultNarrow, config.Default) {
			return nil, false
		}
	}

	arena := &Arena[K, V]{
		owner:        &arenaOwner{},
		keyRange:     config.KeyRange,
		values:       config.Lattice,
		defaultValue: config.Default,
		fingerprint:  config.Fingerprint,
		widenRank:    config.WidenRank,
		narrowRank:   config.NarrowRank,
	}
	empty := newNodePage(arena.owner, nil, 1)
	emptyRef := appendNode(empty, node{kind: emptyNode})
	sealHeader(&empty.header)
	arena.empty = Root[K, V]{ref: emptyRef}
	return arena, true
}

// Empty returns the canonical sealed default root.
func (arena *Arena[K, V]) Empty() Root[K, V] { return arena.empty }

// EmptyHandle returns the canonical sealed default root as an opaque Handle.
// The returned capability belongs only to this Arena and is accepted by Root.
func (arena *Arena[K, V]) EmptyHandle() Handle {
	if arena == nil {
		return Handle{}
	}
	return Handle{ref: arena.empty.ref}
}

// Handle converts one sealed typed Root into its opaque storage handle. It
// rejects foreign and candidate roots, so a caller cannot smuggle a mutable
// root into a published heterogeneous vector.
func (arena *Arena[K, V]) Handle(root Root[K, V]) (Handle, bool) {
	if !arena.Valid(root) {
		return Handle{}, false
	}
	return Handle{ref: root.ref}, true
}

// Root recovers one sealed typed Root from an opaque Handle. The exact Arena
// page authority is checked before the typed capability is returned.
func (arena *Arena[K, V]) Root(handle Handle) (Root[K, V], bool) {
	root := Root[K, V]{ref: handle.ref}
	if !arena.Valid(root) {
		return Root[K, V]{}, false
	}
	return root, true
}

// Owns reports whether handle is a root capability minted by this Arena. It
// deliberately accepts both sealed roots and candidate roots: a heterogeneous
// candidate vector must retain a typed Work's private result until the common
// transaction seal. Readability still requires Root (or the owning Work's
// Root), so Owns grants no access to a candidate value.
//
// Handle has no exported fields. Once this owner/page-kind check succeeds,
// only factor itself could have created its root ref; slot-range and Work-lease
// validation remain the responsibility of the typed conversion that reads or
// publishes it. Avoiding a candidate page-length read here is intentional: a
// foreign Work may be appending to that page concurrently.
func (arena *Arena[K, V]) Owns(handle Handle) bool {
	if arena == nil || handle.ref.zero() || handle.ref.slot < 0 {
		return false
	}
	_, valid := nodePageOf(arena.owner, handle.ref)
	return valid
}

// Default returns the semantic value represented by an absent key.
func (arena *Arena[K, V]) Default() V { return arena.defaultValue }

// Begin opens one isolated, candidate-private construction scope. It is lazy:
// creating Work allocates no storage page. Prepare validates and compacts the
// exact roots retained by its owner without rewriting them; Seal performs the
// sole root rewrite while publishing that prepared closure after Queue.FreezeTerminal.
func (arena *Arena[K, V]) Begin() *Work[K, V] {
	return &Work[K, V]{arena: arena, lease: &workLease{}}
}

// Valid reports whether root is a sealed root of this exact Arena.
func (arena *Arena[K, V]) Valid(root Root[K, V]) bool {
	_, valid := arena.node(root.ref)
	return valid
}

// Count returns the explicit non-default key count and root validity. Count
// uses Go's machine-sized int: it has no Factor-level numeric ceiling.
func (arena *Arena[K, V]) Count(root Root[K, V]) (int, bool) {
	node, valid := arena.node(root.ref)
	if !valid {
		return 0, false
	}
	return node.count, true
}

// Equal reports pointwise lattice equality after exact Arena validation.
func (arena *Arena[K, V]) Equal(left, right Root[K, V]) (bool, bool) {
	if !arena.Valid(left) || !arena.Valid(right) {
		return false, false
	}
	return equalRoots[K, V](arena, arena.values, left.ref, right.ref)
}

// Fingerprint returns Factor's collision-tolerant semantic root summary. It
// is deterministic and equality-consistent, but is never a proof of equality:
// callers must use Equal before merging two matching fingerprints.
func (arena *Arena[K, V]) Fingerprint(root Root[K, V]) (uint64, bool) {
	if !arena.Valid(root) {
		return 0, false
	}
	return rootFingerprint[K, V](arena, arena.fingerprint, root.ref)
}

// Get reads one sealed root. Candidate roots can only be read through their
// owning Work, which makes accidental publication of mutable storage fail
// closed.
func (arena *Arena[K, V]) Get(root Root[K, V], key K) (V, bool, bool) {
	if !arena.admits(key) || !arena.Valid(root) {
		return arena.defaultValue, false, false
	}
	value, present := lookup(arena, root.ref, key)
	if !present {
		return arena.defaultValue, false, true
	}
	return value, present, true
}

// ForEach streams a sealed root in deterministic radix order. It
// never materializes or sorts a whole Factor.
func (arena *Arena[K, V]) ForEach(root Root[K, V], visit func(key K, value V) bool) (completed, valid bool) {
	if !arena.Valid(root) {
		return false, false
	}
	return walk(arena, root.ref, func(key K, value V) bool { return visit(key, value) }), true
}

// ValidLocation accepts only an admitted direct coordinate of this Arena.
func (arena *Arena[K, V]) ValidLocation(location Location[K]) bool {
	return location.owner == arena.owner && arena.admits(location.key)
}

// CompareLocations orders two direct exact locations numerically.
func (arena *Arena[K, V]) CompareLocations(left, right Location[K]) (int, bool) {
	if !arena.ValidLocation(left) || !arena.ValidLocation(right) {
		return 0, false
	}
	return compareKey(left.key, right.key), true
}

func (arena *Arena[K, V]) sameValue(left, right V) bool {
	return sameWith(arena.values, left, right)
}

func (arena *Arena[K, V]) isDefault(value V) bool {
	return arena.sameValue(value, arena.defaultValue)
}

// admits checks the direct finite universe at every raw-key ingress. It
// performs no allocation and never enumerates the universe.
func (arena *Arena[K, V]) admits(key K) bool {
	return arena.keyRange.contains(uint64(key))
}

func sameWith[V any](values lattice.Lattice[V], left, right V) bool {
	return values.Same != nil && values.Same(left, right) || values.Equal(left, right)
}
