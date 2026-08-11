package guard

// Work owns all mutable candidate-local BDD construction state. Its pages are
// never reused or remapped: Seal publishes them once, while Discard drops every
// Work-held reference without leaving a Manager-side history.
type Work struct {
	manager    *Manager
	checkpoint func() bool
	pages      []*page
	owned      map[*page]struct{}
	current    *page
	state      workState

	unique     map[nodeFingerprint][]Guard
	not        map[Guard]Guard
	applyCache map[applyKey]Guard
	ite        map[iteKey]Guard
	restrict   map[restrictKey]Guard
	exists     map[existsKey]Guard
	hashes     map[Guard]uint64

	compareStack []comparePair
	compareSeen  map[comparePair]uint64
	satStack     []satisfiablePair
	satSeen      map[satisfiablePair]uint64
	hashStack    []hashFrame
	readEpoch    uint64
}

// workState records the transaction outcome, independently from each page's
// reader-publication bit. Work is single-writer, so this needs no atomics.
type workState uint8

const (
	workOpen workState = iota
	workPublished
	workDiscarded
)

type nodeFingerprint struct {
	rank uint64
	low  uint64
	high uint64
}

type hashFrame struct {
	guard Guard
	phase uint8
}

type operation uint8

const (
	andOperation operation = iota + 1
	orOperation
)

type applyKey struct {
	operation operation
	left      Guard
	right     Guard
}

type iteKey struct {
	condition Guard
	then      Guard
	otherwise Guard
}

type restrictKey struct {
	guard Guard
	rank  uint64
	value bool
}

type existsKey struct {
	guard Guard
	rank  uint64
}

func (w *Work) requireOpen() {
	if !w.Open() {
		panic("guard: work is published or discarded")
	}
}

// SetCheckpoint installs an opaque epoch-owned liveness probe. Guard has no
// scheduler or context dependency; a false probe merely makes this unsealed
// candidate unusable so its owner can Discard it. It is installed while Work
// is idle and never represents a semantic traversal budget.
func (w *Work) SetCheckpoint(checkpoint func() bool) bool {
	if !w.Open() {
		return false
	}
	w.checkpoint = checkpoint
	return true
}

// Live is the candidate-time counterpart of Open. A cancelled candidate is
// intentionally not sealed, but remains discardable by its owning support
// transaction.
func (w *Work) Live() bool {
	return w.Open() && (w.checkpoint == nil || w.checkpoint())
}

// False returns the candidate's false terminal. It is provided on Work so a
// structural client can construct a complete symbolic partition without
// reaching through Work's private Manager ownership.
func (w *Work) False() Guard {
	w.requireOpen()
	return w.manager.False()
}

// True returns the candidate's true terminal. It is provided on Work so a
// structural client can construct a complete symbolic partition without
// reaching through Work's private Manager ownership.
func (w *Work) True() Guard {
	w.requireOpen()
	return w.manager.True()
}

// Open reports whether w is the live, single-writer construction transaction.
func (w *Work) Open() bool { return w != nil && w.manager != nil && w.state == workOpen }

// Published reports whether w successfully sealed its pages for Manager reads.
// Discarded work is terminal but never published.
func (w *Work) Published() bool {
	return w != nil && w.manager != nil && w.state == workPublished
}

func (w *Work) owns(g Guard) bool {
	if w == nil {
		return false
	}
	if g.manager != w.manager {
		return false
	}
	if isTerminal(g) {
		return g.slot == falseSlot || g.slot == trueSlot
	}
	// A page becomes readable to another Work only after the release store in
	// Seal. Do not even read its slice header before that acquire load: its
	// owner may still append nodes concurrently.
	if g.page.sealed.Load() {
		return uint64(g.slot) < uint64(len(g.page.nodes))
	}
	_, owned := w.owned[g.page]
	if !owned {
		return false
	}
	// Work is single-writer, so its own unsealed page is safe to inspect.
	return uint64(g.slot) < uint64(len(g.page.nodes))
}

func (w *Work) require(g Guard) {
	if !w.owns(g) {
		panic("guard: foreign or unsealed guard")
	}
}

func (w *Work) rank(g Guard) uint64 { return w.manager.rank(g) }

func (w *Work) node(g Guard) node { return g.page.nodes[g.slot] }

func (w *Work) newPage() *page {
	if w.owned == nil {
		w.owned = make(map[*page]struct{})
	}
	p := &page{nodes: make([]node, 0, pageNodeCapacity), lease: &pageLease{}}
	w.pages = append(w.pages, p)
	w.owned[p] = struct{}{}
	w.current = p
	return p
}

func (w *Work) makeNode(rank uint64, low, high Guard) Guard {
	if w.Equivalent(low, high) {
		return low
	}
	if w.unique == nil {
		w.unique = make(map[nodeFingerprint][]Guard)
	}
	if w.hashes == nil {
		w.hashes = make(map[Guard]uint64)
	}
	key := nodeFingerprint{rank: rank, low: w.fingerprint(low), high: w.fingerprint(high)}
	for _, candidate := range w.unique[key] {
		n := w.node(candidate)
		if n.rank == rank && w.Equivalent(n.low, low) && w.Equivalent(n.high, high) {
			return candidate
		}
	}
	if w.current == nil || len(w.current.nodes) == cap(w.current.nodes) {
		w.newPage()
	}
	slot := uint32(len(w.current.nodes))
	guard := Guard{manager: w.manager, page: w.current, slot: slot}
	w.current.nodes = append(w.current.nodes, node{rank: rank, low: low, high: high})
	w.unique[key] = append(w.unique[key], guard)
	w.hashes[guard] = mixFingerprint(mixFingerprint(mixFingerprint(0x517cc1b727220a95, rank), key.low), key.high)
	return guard
}

// Literal constructs the positive literal only when atom belongs to the
// Manager's presealed universe.
func (w *Work) Literal(atom Atom) (Guard, bool) {
	w.requireOpen()
	if !w.Live() {
		return Guard{}, false
	}
	rank, exists := w.manager.atoms[atom]
	if !exists {
		return Guard{}, false
	}
	return w.makeNode(rank, w.manager.False(), w.manager.True()), true
}

// Seal freezes every candidate page, then drops all mutable tables and page
// inventory. Published roots retain precisely the pages reachable through
// their node edges; Manager retains none.
func (w *Work) Seal() {
	w.requireOpen()
	if !w.Live() {
		return
	}
	for _, p := range w.pages {
		p.sealed.Store(true)
	}
	w.state = workPublished
	w.pages = nil
	w.current = nil
	w.owned = nil
	w.unique = nil
	w.not = nil
	w.applyCache = nil
	w.ite = nil
	w.restrict = nil
	w.exists = nil
	w.hashes = nil
	w.hashStack = nil
	w.compareStack = nil
	w.compareSeen = nil
	w.satStack = nil
	w.satSeen = nil
}

// Discard drops all candidate-local construction state. Its unsealed pages
// remain unreadable and become collectible unless retained by an invalid local
// Guard, which Manager.Valid will reject.
func (w *Work) Discard() {
	if !w.Open() {
		return
	}
	w.state = workDiscarded
	w.pages = nil
	w.current = nil
	w.owned = nil
	w.unique = nil
	w.not = nil
	w.applyCache = nil
	w.ite = nil
	w.restrict = nil
	w.exists = nil
	w.hashes = nil
	w.hashStack = nil
	w.compareStack = nil
	w.compareSeen = nil
	w.satStack = nil
	w.satSeen = nil
}

// fingerprint is a Work-local structural cache. Its bucket is never trusted as
// equality: makeNode always follows it with exact iterative comparison.
func (w *Work) fingerprint(root Guard) uint64 {
	if w.hashes == nil {
		w.hashes = make(map[Guard]uint64)
	}
	if value, exists := w.hashes[root]; exists {
		return value
	}
	if isTerminal(root) {
		value := uint64(0x9e3779b97f4a7c15)
		if terminalValue(root) {
			value = 0xc2b2ae3d27d4eb4f
		}
		w.hashes[root] = value
		return value
	}
	w.hashStack = append(w.hashStack[:0], hashFrame{guard: root})
	for len(w.hashStack) != 0 {
		current := &w.hashStack[len(w.hashStack)-1]
		if _, exists := w.hashes[current.guard]; exists {
			w.hashStack = w.hashStack[:len(w.hashStack)-1]
			continue
		}
		if isTerminal(current.guard) {
			value := uint64(0x9e3779b97f4a7c15)
			if terminalValue(current.guard) {
				value = 0xc2b2ae3d27d4eb4f
			}
			w.hashes[current.guard] = value
			w.hashStack = w.hashStack[:len(w.hashStack)-1]
			continue
		}
		n := w.node(current.guard)
		switch current.phase {
		case 0:
			current.phase = 1
			if _, exists := w.hashes[n.low]; !exists {
				w.hashStack = append(w.hashStack, hashFrame{guard: n.low})
			}
		case 1:
			current.phase = 2
			if _, exists := w.hashes[n.high]; !exists {
				w.hashStack = append(w.hashStack, hashFrame{guard: n.high})
			}
		default:
			low, high := w.hashes[n.low], w.hashes[n.high]
			value := mixFingerprint(mixFingerprint(mixFingerprint(0x517cc1b727220a95, n.rank), low), high)
			w.hashes[current.guard] = value
			w.hashStack = w.hashStack[:len(w.hashStack)-1]
		}
	}
	return w.hashes[root]
}

func mixFingerprint(value, next uint64) uint64 {
	return (value ^ next) * 0x100000001b3
}
