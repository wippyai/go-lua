package typestate

import (
	"sort"

	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
)

// Key is one exact structural resource origin in the homogeneous Typestate
// Factor family.
type Key struct{ Resource ResourceOrigin }

func (k Key) validFor(source *proglink.Link) bool { return k.Resource.validFor(source) }

// Duty is Typestate's complete cleanup-obligation state. It is State-bound
// semantic data, not a protocol symbol or persistent manifest coordinate.
type Duty uint8

const (
	DutyInvalid Duty = iota
	DutyLocal
	DutyHandoff
	DutyDischarged
	DutyUnknown
)

func (d Duty) Valid() bool { return d >= DutyLocal && d <= DutyUnknown }

// Multiplicity is P({0,1,many}) for one exact holder origin.
type Multiplicity uint8

const (
	CountZero Multiplicity = 1 << iota
	CountOne
	CountMany
)

const allCounts = CountZero | CountOne | CountMany

func (m Multiplicity) Valid() bool { return m != 0 && m&^allCounts == 0 }
func (m Multiplicity) Has(count Multiplicity) bool {
	return count.Valid() && m&count == count
}

// Entry is one correlated protocol-state, duty, holder-origin alternative.
// Resource remains solely in the Factor key.
type Entry struct {
	State  StateCoordinate
	Duty   Duty
	Holder HolderOrigin
	Count  Multiplicity
}

func (e Entry) wellFormedFor(universe *universe) bool {
	return universe != nil && e.State.validFor(universe) && e.Duty.Valid() &&
		e.Holder.validFor(universe.source) && e.Count.Valid()
}

// Relation is Bottom, a finite normalized set of declared coordinate masks,
// or Top. All keys in one Factor family share this carrier and owner.
type Relation struct {
	universe *universe
	top      bool
	cells    []cell
}

// cell is the complete hot payload. State/duty/holder live once in Schema.
type cell struct {
	coordinate uint32
	resource   keyspace.ContentID
	count      Multiplicity
}

func bottomRelation(universe *universe) Relation { return Relation{universe: universe} }
func topRelation(universe *universe) Relation    { return Relation{universe: universe, top: true} }
func (r Relation) IsBottom() bool                { return !r.top && len(r.cells) == 0 }
func (r Relation) IsTop() bool                   { return r.top }

// Entries returns the semantic tuples in canonical schema order.
func (r Relation) Entries() []Entry {
	if r.universe == nil || r.top {
		return nil
	}
	entries := make([]Entry, len(r.cells))
	for i, current := range r.cells {
		_, coordinate, ok := Schema{universe: r.universe}.CoordinateAt(int(current.coordinate - 1))
		if !ok {
			return nil
		}
		entries[i] = Entry{State: coordinate.State, Duty: coordinate.Duty, Holder: coordinate.Holder, Count: current.count}
	}
	return entries
}

func normalizeRelation(schema Schema, key Key, entries []Entry) (Relation, bool) {
	universe := schema.universe
	if len(entries) == 0 {
		return bottomRelation(universe), true
	}
	cells := make([]cell, len(entries))
	resourceID := key.Resource.ContentID()
	for i, entry := range entries {
		if !entry.wellFormedFor(universe) {
			return Relation{}, false
		}
		coordinate, ok := schema.coordinate(key, Coordinate{State: entry.State, Duty: entry.Duty, Holder: entry.Holder})
		if !ok {
			return Relation{}, false
		}
		cells[i] = cell{coordinate: coordinate, resource: resourceID, count: entry.Count}
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].coordinate < cells[j].coordinate })
	normalized := cells[:0]
	for _, current := range cells {
		if len(normalized) > 0 && normalized[len(normalized)-1].coordinate == current.coordinate {
			normalized[len(normalized)-1].count |= current.count
			continue
		}
		normalized = append(normalized, current)
	}
	return Relation{universe: universe, cells: normalized}, true
}

func equalRelation(left, right Relation) bool {
	if left.universe != right.universe || left.top != right.top || len(left.cells) != len(right.cells) {
		return false
	}
	for i := range left.cells {
		if left.cells[i] != right.cells[i] {
			return false
		}
	}
	return true
}

func lessRelation(left, right Relation) bool {
	if left.universe == nil || left.universe != right.universe {
		return false
	}
	if left.top || right.top {
		return right.top
	}
	li, ri := 0, 0
	for li < len(left.cells) && ri < len(right.cells) {
		switch {
		case left.cells[li].coordinate == right.cells[ri].coordinate:
			if left.cells[li].count&^right.cells[ri].count != 0 {
				return false
			}
			li++
			ri++
		case left.cells[li].coordinate < right.cells[ri].coordinate:
			return false
		default:
			ri++
		}
	}
	return li == len(left.cells)
}

func joinRelation(left, right Relation) Relation {
	if left.top || right.top {
		return topRelation(left.universe)
	}
	if left.IsBottom() {
		return right
	}
	if right.IsBottom() {
		return left
	}
	cells := make([]cell, 0, len(left.cells)+len(right.cells))
	li, ri := 0, 0
	for li < len(left.cells) && ri < len(right.cells) {
		switch {
		case left.cells[li].coordinate == right.cells[ri].coordinate:
			current := left.cells[li]
			current.count |= right.cells[ri].count
			cells = append(cells, current)
			li++
			ri++
		case left.cells[li].coordinate < right.cells[ri].coordinate:
			cells = append(cells, left.cells[li])
			li++
		default:
			cells = append(cells, right.cells[ri])
			ri++
		}
	}
	cells = append(cells, left.cells[li:]...)
	cells = append(cells, right.cells[ri:]...)
	return Relation{universe: left.universe, cells: cells}
}

func meetRelation(left, right Relation) Relation {
	if left.top {
		return right
	}
	if right.top {
		return left
	}
	cells := make([]cell, 0, minRelation(len(left.cells), len(right.cells)))
	li, ri := 0, 0
	for li < len(left.cells) && ri < len(right.cells) {
		switch {
		case left.cells[li].coordinate == right.cells[ri].coordinate:
			current := left.cells[li]
			current.count &= right.cells[ri].count
			if current.count.Valid() {
				cells = append(cells, current)
			}
			li++
			ri++
		case left.cells[li].coordinate < right.cells[ri].coordinate:
			li++
		default:
			ri++
		}
	}
	return Relation{universe: left.universe, cells: cells}
}

func minRelation(left, right int) int {
	if left < right {
		return left
	}
	return right
}
