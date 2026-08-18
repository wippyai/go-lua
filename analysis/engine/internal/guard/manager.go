// Package guard implements exact reduced ordered binary decision diagrams for
// symbolic solver guards. A Manager owns the atom order while each candidate
// owns its BDD pages through a disposable Work.
package guard

import (
	"errors"
	"sync/atomic"
)

// Atom identifies one presealed caller-defined proposition. Its numeric value
// is its global order; callers must provide every admissible atom to New.
type Atom uint64

// Guard is an immutable direct page/slot handle authorized by one Manager.
// Terminals have a nil page and slots falseSlot or trueSlot. Nonterminal pages
// become readable only after their Work has sealed them.
type Guard struct {
	manager *Manager
	page    *page
	slot    uint32
}

const (
	falseSlot uint32 = iota
	trueSlot
)

var (
	ErrDuplicateAtom = errors.New("guard: duplicate atom in order")
	ErrUnsortedAtom  = errors.New("guard: atoms must be strictly ascending")
)

// Manager deliberately owns neither BDD nodes nor operation caches, so it
// cannot retain candidate history or snapshots. Its atom order is fixed for
// one compiled carrier generation; topology reformation builds a fresh
// Manager rather than mutating a live Guard universe.
type Manager struct {
	atoms map[Atom]uint64
	order []Atom
	all   Scope
}

// pageNodeCapacity is physical storage granularity, not an analysis bound.
// A Work spills to fresh pages indefinitely as needed.
//
// A transaction's first page is firstPageNodeCapacity nodes and each further
// page grows by pageGrowthFactor up to pageNodeCapacity. Page size describes
// one transaction's storage, and transaction sizes span orders of magnitude:
// the read path constructs one or two nodes per candidate while a construction
// pass fills page after page. Growth serves both from one geometry.
const (
	pageNodeCapacity      = 256
	firstPageNodeCapacity = 4
	pageGrowthFactor      = 4
)

type page struct {
	nodes  []node
	lease  *pageLease
	sealed atomic.Bool
}

// pageLease is an acyclic page-owned retention marker used only to make page
// reachability observable in tests without relying on finalization of a
// self-referential node graph.
type pageLease struct{ marker byte }

type node struct {
	rank uint64
	low  Guard
	high Guard
}

// New defines one finite, strictly ascending atom universe. It copies its
// input and never changes its order afterwards.
func New(order []Atom) (*Manager, error) {
	manager := &Manager{
		atoms: make(map[Atom]uint64, len(order)),
		order: append([]Atom(nil), order...),
	}
	for rank, atom := range manager.order {
		if rank > 0 && atom == manager.order[rank-1] {
			return nil, ErrDuplicateAtom
		}
		if rank > 0 && atom < manager.order[rank-1] {
			return nil, ErrUnsortedAtom
		}
		manager.atoms[atom] = uint64(rank)
	}
	all := make([]uint64, len(manager.order))
	for index := range all {
		all[index] = uint64(index)
	}
	manager.all = Scope{value: &scope{manager: manager, ranks: all, sealed: true}}
	return manager, nil
}

// False and True are sealed manager terminals with no page allocation.
func (m *Manager) False() Guard { return Guard{manager: m, slot: falseSlot} }
func (m *Manager) True() Guard  { return Guard{manager: m, slot: trueSlot} }

// NewWork starts one private candidate construction space. Work is
// single-writer; its sealed Guards support concurrent lock-free reads through
// Manager methods after ordinary caller publication synchronization.
func (m *Manager) NewWork() *Work {
	return &Work{manager: m}
}

func (m *Manager) validSealed(g Guard) bool {
	if m == nil || g.manager != m {
		return false
	}
	if g.page == nil {
		return g.slot == falseSlot || g.slot == trueSlot
	}
	return g.page.sealed.Load() && uint64(g.slot) < uint64(len(g.page.nodes))
}

// Valid reports whether g is a sealed, readable guard owned by m.
func (m *Manager) Valid(g Guard) bool { return m.validSealed(g) }

// Rank returns atom's fixed position in this Manager's sealed order.  It is
// the only order authority exposed to neighbouring symbolic storage: callers
// must not infer a decision order from an Atom's numeric spelling.
func (m *Manager) Rank(atom Atom) (uint64, bool) {
	if m == nil {
		return 0, false
	}
	rank, ok := m.atoms[atom]
	return rank, ok
}

func isTerminal(g Guard) bool { return g.page == nil }

func terminalValue(g Guard) bool { return g.slot == trueSlot }

func (m *Manager) rank(g Guard) uint64 {
	if isTerminal(g) {
		return ^uint64(0)
	}
	return g.page.nodes[g.slot].rank
}

func (m *Manager) atom(rank uint64) Atom { return m.order[rank] }

// AtomAt returns the canonical atom at a known sealed order rank. It is used
// only by symbolic storage rebuilding an already-admitted decision; callers
// cannot change Manager order through this read-only lookup.
func (m *Manager) AtomAt(rank uint64) (Atom, bool) {
	if m == nil || rank >= uint64(len(m.order)) {
		return 0, false
	}
	return m.order[rank], true
}

// Scope is an immutable finite coordinate namespace owned by one Manager.
// It is intentionally a capability rather than an atom slice: hot carrier
// operations may compare or validate scopes but cannot enumerate or alter
// their coordinates.
type Scope struct{ value *scope }

type scope struct {
	manager *Manager
	sealed  bool
	// ranks is the immutable, strictly ascending Manager-rank set owned by
	// this scope. It is deliberately compact: a scope remains valid when its
	// Manager later gains an appended atom, while the new rank is absent until
	// a new scope explicitly includes it.
	ranks []uint64
}

// AllScope returns the Manager's complete presealed coordinate universe.
func (m *Manager) AllScope() Scope {
	if m == nil {
		return Scope{}
	}
	return m.all
}

// SealScope creates one exact finite coordinate namespace. Atom spelling is
// admitted only at this cold sealing boundary; execution receives Scope.
func (m *Manager) SealScope(atoms []Atom) (Scope, bool) {
	if m == nil {
		return Scope{}, false
	}
	ranks := make([]uint64, len(atoms))
	for index, atom := range atoms {
		rank, exists := m.atoms[atom]
		if !exists || index > 0 && atoms[index-1] >= atom {
			return Scope{}, false
		}
		ranks[index] = rank
	}
	return Scope{value: &scope{manager: m, ranks: ranks, sealed: true}}, true
}

// Valid reports whether this is a Manager-issued immutable scope.
func (s Scope) Valid() bool {
	return s.value != nil && s.value.manager != nil && s.value.sealed
}

// Manager returns the one guard universe for this scope.
func (s Scope) Manager() *Manager {
	if !s.Valid() {
		return nil
	}
	return s.value.manager
}

// Same proves exact coordinate-scope identity. Equal-looking scopes remain
// distinct interfaces unless they were issued as the same sealed scope.
func (s Scope) Same(other Scope) bool { return s.Valid() && s.value == other.value }

func (s Scope) containsRank(rank uint64) bool {
	if !s.Valid() {
		return false
	}
	return hasRank(s.value.ranks, rank)
}

func (s Scope) contains(atom Atom) bool {
	if !s.Valid() {
		return false
	}
	rank, exists := s.value.manager.atoms[atom]
	return exists && hasRank(s.value.ranks, rank)
}

// rankSearch returns the first index whose rank is at least want. It is kept
// local to the guard package so hot scope membership checks do not allocate a
// closure through sort.Search.
func rankSearch(ranks []uint64, want uint64) int {
	low, high := 0, len(ranks)
	for low < high {
		middle := low + (high-low)/2
		if ranks[middle] < want {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low
}

func hasRank(ranks []uint64, want uint64) bool {
	index := rankSearch(ranks, want)
	return index < len(ranks) && ranks[index] == want
}

// Contains reports whether a sealed Guard mentions only this scope's
// coordinates. It is a cold boundary validation used when a State enters a
// scoped carrier; hot execution relies on the sealed plan proof instead.
func (s Scope) Contains(root Guard) bool {
	if !s.Valid() || !s.value.manager.Valid(root) {
		return false
	}
	completed, valid := s.value.manager.Fold(root, func(_ Guard, view Decomposition) bool {
		return view.Terminal || s.contains(view.Atom)
	})
	return completed && valid
}
