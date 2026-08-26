// Package operand owns the value types a reducer's operands arrive in.
//
// A reducer is lattice judgment: it stays authored domain source forever,
// while the executor that fills its parameters is protocol and does not. The
// types on that boundary therefore belong to neither side. They are stated
// here so an authored fold and the engine that invokes it name the same
// values without the fold importing the machinery that produced them.
//
// The surface is read-only by construction. Nothing here opens a cursor,
// stages a write, or holds a ticket: a vector reports its validity, width and
// cells, and a cell reports its value, its tag and its presence. Delivery is
// the engine's own statement and stays in the engine.
package operand

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// SummaryVector is one partition row's whole cell vector. It is a view over
// the Binding-owned observation storage: constructing one allocates nothing,
// and At is a direct index that neither compacts nor reorders.
//
// Absence is preserved per cell. A coordinate whose value was deleted stays a
// cell of the vector and reports present false, so the reader keeps the
// correlation between a cell's position and the coordinate the owner declared
// at that position. Compacting the absent cells away would silently renumber
// every later cell.
type SummaryVector[V any] struct {
	view factbinding.Observation[V]
	// members is the second backing: the cells of a nested member set, read
	// one ordinal at a time at each member's own exact coordinate rather than
	// delivered by a Factor cursor. See NewMemberVector for why that is a
	// backing of this view and not a view of its own.
	members []MemberCell[V]
	width   uint16
	open    bool
}

// MemberCell is one cell of a member-set vector: the fact read at that
// member's own coordinate, and whether that coordinate holds one. Absence is
// carried per cell for the same reason a Factor-backed vector carries it - the
// position of a cell is the ordinal its owner declared it at, and compacting
// an absent cell away would renumber every later one.
type MemberCell[V any] struct {
	Value   V
	Present bool
	// Region is the support this cell was observed over. A vector's cells are
	// read one coordinate at a time and each answers over what its own read
	// proved, so the conclusion folded from them holds over the conjunction of
	// their supports and not over the window the invocation opened.
	Region support.Mask
}

// NewObservationVector views one live Binding-owned observation as the vector
// its reader is declared to receive. The width is the cell count the cursor
// itself reported: a delivery that cannot state one has no vector to hand
// over, and a count no ordinal can address is refused here rather than
// truncated into one.
func NewObservationVector[V any](view factbinding.Observation[V], width int) (SummaryVector[V], bool) {
	if width < 0 || width > int(^uint16(0)) {
		return SummaryVector[V]{}, false
	}
	return SummaryVector[V]{view: view, width: uint16(width), open: true}, true
}

// NewMemberVector views one caller-owned member-set cell slice as the vector
// its reader is declared to receive.
//
// A nested member set is a closed denominator the owner itself publishes -
// its MemberCount and MemberAt ARE the denominator - so a read that spans it
// is a whole-vector read, and the declaration that says so is a Summary read.
// What differs is only where the cells come from: a Factor-backed summary read
// delivers a view over Binding-owned observation storage, while a member set
// is read one ordinal at a time through each member's own exact coordinate.
//
// The reader must not be able to tell. A many-valued input is ONE vector
// argument under the reducer call shape, so a second vector type would split
// every fold that consumes one into two spellings of the same parameter. The
// view therefore carries a second backing rather than growing a sibling.
//
// The slice is caller-owned and lives for the invocation, which is what keeps
// a warm member-set read allocation-free: a family sizes it once at its sealed
// member width and refills it per invocation.
func NewMemberVector[V any](cells []MemberCell[V]) (SummaryVector[V], bool) {
	if cells == nil || len(cells) > int(^uint16(0)) {
		return SummaryVector[V]{}, false
	}
	return SummaryVector[V]{members: cells, width: uint16(len(cells)), open: true}, true
}

// Valid reports whether the vector still belongs to its live read cursor, or,
// for a member set, to the caller-owned cells it was opened over.
func (vector SummaryVector[V]) Valid() bool {
	if !vector.open {
		return false
	}
	if vector.members != nil {
		return len(vector.members) == int(vector.width)
	}
	return vector.view.Valid()
}

// Count is the declared cell width of this vector, absent cells included.
func (vector SummaryVector[V]) Count() int {
	if !vector.open {
		return 0
	}
	return int(vector.width)
}

// At returns one cell in sealed declaration order. The three results are the
// typed value, whether that coordinate holds one, and whether the index names
// a cell at all: an absent cell is an available cell with no value, while an
// out-of-range index is not a cell.
func (vector SummaryVector[V]) At(index int) (V, bool, bool) {
	var zero V
	if !vector.open || index < 0 || index >= int(vector.width) {
		return zero, false, false
	}
	if vector.members != nil {
		if len(vector.members) != int(vector.width) {
			return zero, false, false
		}
		cell := vector.members[index]
		return cell.Value, cell.Present, true
	}
	entry, ok := vector.view.At(index)
	if !ok {
		return zero, false, false
	}
	value, present := entry.Read()
	return value, present, true
}
