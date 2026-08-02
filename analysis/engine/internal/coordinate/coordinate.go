// Package coordinate owns the Solver-private, demanded coordinate table over
// one immutable project Link. It does not reconstruct a graph, invent source
// identities, or enumerate Link possibilities.
package coordinate

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

// Coordinate is the sole Solver-private representation of one demanded
// semantic location. Its table/slot pair is operational provenance; its
// canonical order comes from the existing Link Candidate, shard, and Program
// Term retained by that Table.  Keeping the pair together prevents a raw
// discovery-order integer from becoming an order authority in dependency.
//
// A scalar cannot losslessly order both roots and future lazy
// Candidate×Term locations: a Candidate is itself a finite structural tuple,
// and that product must not be pre-materialized merely to allocate ranks.
// Coordinate therefore owns the one direct comparison operation. It is a
// statically bound method, not a caller-supplied comparator or a second
// source-identity path.
type Coordinate struct {
	table *Table
	slot  uint32 // one-based row slot; zero is invalid
}

// Valid reports whether coordinate is an existing member of its one owning
// demanded table. It is intentionally not a semantic query.
func (coordinate Coordinate) Valid() bool {
	return coordinate.table != nil && coordinate.table.owns(coordinate)
}

// Slot returns Coordinate's table-local storage position. It is an
// operational lookup capability, never semantic identity or ordering.
// Keeping this on the handle lets an immutable State retain roots in direct
// table-slot order without retaining a second coordinate index.
func (coordinate Coordinate) Slot() (int, bool) {
	if coordinate.table == nil {
		return 0, false
	}
	return coordinate.table.Slot(coordinate)
}

// Compare provides Coordinate's canonical order to the dependency hot path.
// Cross-table coordinates are ineligible for one Solver generation; the
// Table owner sequence merely keeps the comparison total if an internal
// misuse reaches a generic container. It is never a semantic order.
func (coordinate Coordinate) Compare(other Coordinate) int {
	if coordinate.table != other.table {
		return compareOwner(coordinate.table, other.table)
	}
	if coordinate.table == nil {
		return 0
	}
	order, ok := coordinate.table.Compare(coordinate, other)
	if !ok {
		return 0
	}
	return order
}

// Table materializes only demanded locations. A location is either one
// executable Program term under a shard's Entry activation, or one Program
// term at one exact Link body Candidate. Link Candidates remain the invocation
// identity; Program Terms remain the sole source identity.
type Table struct {
	link  *link.Link
	owner uint64

	byKey map[key]Coordinate
	rows  []row

	ordered      []Coordinate
	orderCurrent bool
}

type key struct {
	candidate link.Candidate
	shard     link.Shard
	term      program.Term
}

type row struct {
	coordinate Coordinate
	key        key
}

// nextTableOwner separates coordinates from different Solver Tables. It is
// only a fail-closed provenance tiebreak for invalid cross-Table use; valid
// dependency work never observes it as semantic order.
var nextTableOwner atomic.Uint64

// New binds an initially empty demanded table to one sealed Link.
func New(source *link.Link) (*Table, bool) {
	if source == nil || !source.ContentID().Available() {
		return nil, false
	}
	owner, ok := mintTableOwner()
	if !ok {
		return nil, false
	}
	return &Table{
		link:  source,
		owner: owner,
		byKey: make(map[key]Coordinate),
	}, true
}

// mintTableOwner reserves one process-local provenance namespace without
// wrapping. It cannot change semantic ordering: a valid dependency generation
// has exactly one Table. Refusing exhaustion prevents a foreign-coordinate
// tiebreak from ever aliasing a prior Table.
func mintTableOwner() (uint64, bool) {
	return mintOwner(&nextTableOwner)
}

func mintOwner(sequence *atomic.Uint64) (uint64, bool) {
	if sequence == nil {
		return 0, false
	}
	for {
		current := sequence.Load()
		if current == ^uint64(0) {
			return 0, false
		}
		next := current + 1
		if sequence.CompareAndSwap(current, next) {
			return next, true
		}
	}
}

// InternRoot materializes an executable term under shard's one explicit Entry
// activation. Nested-function terms and non-executable Program identities are
// rejected; an ordinary entry-body occurrence needs no second point plane.
func (table *Table) InternRoot(shard link.Shard, term program.Term) (Coordinate, bool) {
	if table == nil || table.link == nil {
		return Coordinate{}, false
	}
	p, ok := table.link.Program(shard)
	if !ok || p == nil {
		return Coordinate{}, false
	}
	entry, ok := p.Entry()
	if !ok {
		return Coordinate{}, false
	}
	if activation, ok := p.Activation(term); !ok || activation != entry {
		return Coordinate{}, false
	}
	return table.intern(key{shard: shard, term: term})
}

// InternCandidate materializes a term only under the exact selected Link
// body Candidate. It validates the selected shard/body through Link and the
// term's invocation body through Program.Activation; no caller can supply a
// reconstructed path, target, or local identity.
func (table *Table) InternCandidate(candidate link.Candidate, shard link.Shard, term program.Term) (Coordinate, bool) {
	if table == nil || table.link == nil || term == 0 {
		return Coordinate{}, false
	}
	candidateShard, body, ok := table.link.CandidateBody(candidate)
	if !ok || candidateShard != shard {
		return Coordinate{}, false
	}
	p, ok := table.link.Program(shard)
	if !ok || p == nil {
		return Coordinate{}, false
	}
	activation, ok := p.Activation(term)
	if !ok || activation != body {
		return Coordinate{}, false
	}
	return table.intern(key{candidate: candidate, shard: shard, term: term})
}

func (table *Table) intern(item key) (Coordinate, bool) {
	if table == nil || !table.valid(item) {
		return Coordinate{}, false
	}
	if coordinate, found := table.byKey[item]; found {
		return coordinate, true
	}
	if uint64(len(table.rows)) >= uint64(^uint32(0)) {
		return Coordinate{}, false
	}
	coordinate := Coordinate{table: table, slot: uint32(len(table.rows) + 1)}
	table.byKey[item] = coordinate
	table.rows = append(table.rows, row{coordinate: coordinate, key: item})
	table.orderCurrent = false
	return coordinate, true
}

// Semantic exposes the exact existing Link Candidate (zero for a root),
// shard, and Program term behind a valid handle. It is intentionally private
// to the engine's internal package graph.
func (table *Table) Semantic(coordinate Coordinate) (link.Candidate, link.Shard, program.Term, bool) {
	item, ok := table.item(coordinate)
	if !ok {
		return link.Candidate{}, 0, 0, false
	}
	return item.candidate, item.shard, item.term, true
}

// Compare gives the canonical semantic order: root before Candidate, then
// Link's Candidate order, then shard and Program Term. It is independent of
// demand/intern order.
func (table *Table) Compare(left, right Coordinate) (int, bool) {
	first, ok := table.item(left)
	if !ok {
		return 0, false
	}
	second, ok := table.item(right)
	if !ok {
		return 0, false
	}
	return table.compare(first, second)
}

// Count is the number of materialized demanded locations, never a potential
// Candidate×Term product.
func (table *Table) Count() int {
	if table == nil {
		return 0
	}
	return len(table.rows)
}

// Slot returns the table-local dense storage ordinal for one owned handle.
// It is operational only: callers must never serialize, compare, or retain it
// as semantic identity. The future transaction uses it to index a private
// root slice without turning global handle provenance into a sparse array.
func (table *Table) Slot(coordinate Coordinate) (int, bool) {
	if table == nil || coordinate.table != table || coordinate.slot == 0 {
		return 0, false
	}
	index := int(coordinate.slot - 1)
	if index < 0 || index >= len(table.rows) || table.rows[index].coordinate != coordinate {
		return 0, false
	}
	return index, true
}

// OrderedAt returns a materialized location in semantic order.
func (table *Table) OrderedAt(index int) (Coordinate, bool) {
	if table == nil || index < 0 || index >= len(table.rows) {
		return Coordinate{}, false
	}
	table.ensureOrder()
	return table.ordered[index], true
}

func (table *Table) valid(item key) bool {
	if table == nil || table.link == nil || item.shard == 0 || item.term == 0 {
		return false
	}
	if item.candidate == (link.Candidate{}) {
		p, ok := table.link.Program(item.shard)
		if !ok || p == nil {
			return false
		}
		entry, ok := p.Entry()
		if !ok {
			return false
		}
		activation, ok := p.Activation(item.term)
		return ok && activation == entry
	}
	selectedShard, body, ok := table.link.CandidateBody(item.candidate)
	if !ok || selectedShard != item.shard {
		return false
	}
	p, ok := table.link.Program(item.shard)
	if !ok || p == nil {
		return false
	}
	activation, ok := p.Activation(item.term)
	return ok && activation == body
}

func (table *Table) item(coordinate Coordinate) (key, bool) {
	if table == nil || coordinate.table != table || coordinate.slot == 0 {
		return key{}, false
	}
	index := int(coordinate.slot - 1)
	if index < 0 || index >= len(table.rows) {
		return key{}, false
	}
	item := table.rows[index]
	if item.coordinate != coordinate {
		return key{}, false
	}
	return item.key, true
}

func (table *Table) owns(coordinate Coordinate) bool {
	if table == nil || coordinate.table != table || coordinate.slot == 0 {
		return false
	}
	index := int(coordinate.slot - 1)
	return index >= 0 && index < len(table.rows) && table.rows[index].coordinate == coordinate
}

func compareOwner(left, right *Table) int {
	if left == right {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if left.owner < right.owner {
		return -1
	}
	return 1
}

func (table *Table) compare(left, right key) (int, bool) {
	leftRoot := left.candidate == (link.Candidate{})
	rightRoot := right.candidate == (link.Candidate{})
	if leftRoot != rightRoot {
		if leftRoot {
			return -1, true
		}
		return 1, true
	}
	if !leftRoot {
		order, ok := table.link.CompareCandidate(left.candidate, right.candidate)
		if !ok {
			return 0, false
		}
		if order != 0 {
			return order, true
		}
	}
	if left.shard < right.shard {
		return -1, true
	}
	if left.shard > right.shard {
		return 1, true
	}
	if left.term < right.term {
		return -1, true
	}
	if left.term > right.term {
		return 1, true
	}
	return 0, true
}

func (table *Table) ensureOrder() {
	if table == nil || table.orderCurrent {
		return
	}
	if cap(table.ordered) < len(table.rows) {
		table.ordered = make([]Coordinate, len(table.rows))
	} else {
		table.ordered = table.ordered[:len(table.rows)]
	}
	for index, item := range table.rows {
		table.ordered[index] = item.coordinate
	}
	sort.Slice(table.ordered, func(left, right int) bool {
		first, _ := table.item(table.ordered[left])
		second, _ := table.item(table.ordered[right])
		order, ok := table.compare(first, second)
		return ok && order < 0
	})
	table.orderCurrent = true
}
