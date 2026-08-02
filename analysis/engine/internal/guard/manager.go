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
}

// pageNodeCapacity is physical storage granularity, not an analysis bound.
// A Work spills to fresh pages indefinitely as needed.
const pageNodeCapacity = 256

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
	return manager, nil
}

// False and True are sealed manager terminals with no page allocation.
func (m *Manager) False() Guard { return Guard{manager: m, slot: falseSlot} }
func (m *Manager) True() Guard  { return Guard{manager: m, slot: trueSlot} }

// NewWork starts one private candidate construction space. Work is
// single-writer; its sealed Guards support concurrent lock-free reads through
// Manager methods after ordinary caller publication synchronization.
func (m *Manager) NewWork() *Work {
	return &Work{
		manager:     m,
		owned:       make(map[*page]struct{}),
		unique:      make(map[nodeFingerprint][]Guard),
		not:         make(map[Guard]Guard),
		applyCache:  make(map[applyKey]Guard),
		ite:         make(map[iteKey]Guard),
		restrict:    make(map[restrictKey]Guard),
		exists:      make(map[existsKey]Guard),
		hashes:      make(map[Guard]uint64),
		compareSeen: make(map[comparePair]uint64),
		satSeen:     make(map[satisfiablePair]uint64),
	}
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

func isTerminal(g Guard) bool { return g.page == nil }

func terminalValue(g Guard) bool { return g.slot == trueSlot }

func (m *Manager) rank(g Guard) uint64 {
	if isTerminal(g) {
		return ^uint64(0)
	}
	return g.page.nodes[g.slot].rank
}

func (m *Manager) atom(rank uint64) Atom { return m.order[rank] }
