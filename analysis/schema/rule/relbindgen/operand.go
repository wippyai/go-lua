package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/identity"
)

// The bridge between a delivered span and the operand vocabulary an owner fold
// reads.
//
// A span is how the relational frame delivers a many-valued read, and
// operand.SelectedCell and operand.SummaryVector are how the analyzer's folds
// have always received one. The operand vocabulary is the surviving one, so a
// fold keeps its signature and the binding materializes what it asks for.
//
// A span holds value tokens, not values, so materializing decodes and cannot
// be a view. What it can be is allocation-free once warm: a caller sizes the
// storage at its sealed width and refills it, which is the reuse
// operand.NewMemberVector is written to accept.

// Cells is reusable storage for materializing one delivered span as the
// selected cells a fold reads. It is one caller's, not one binding's: solve
// local workers must not share it.
type Cells[T any] struct {
	rows []operand.SelectedCell[T]
}

// NewCells reserves storage for one caller's materializations.
func NewCells[T any](reserve int) (*Cells[T], bool) {
	if reserve < 0 {
		return nil, false
	}
	return &Cells[T]{rows: make([]operand.SelectedCell[T], 0, reserve)}, true
}

// Fill materializes the delivered span as selected cells. Each cell's tag is
// the owner tag the caller resolves from the row the span says that position
// carries, so the correlation between a delivered row and the owner identity
// a fold looks it up by is stated by the owner and never by this layer.
//
// A cell's Region is left as it comes. It is the authenticated support a
// routed write stages against, and a binding publishes through the proposal
// buffer instead, so nothing on this path reads one.
func (cells *Cells[T]) Fill(span Span[T], tag func(identity.ContentID) (uint64, bool)) ([]operand.SelectedCell[T], bool) {
	if cells == nil || tag == nil {
		return nil, false
	}
	cells.rows = cells.rows[:0]
	for index := 0; index < span.Len(); index++ {
		row, ok := span.RowKeyAt(index)
		if !ok {
			return nil, false
		}
		owned, resolved := tag(row)
		if !resolved {
			return nil, false
		}
		value, present, available := span.At(index)
		if !available {
			return nil, false
		}
		cells.rows = append(cells.rows, operand.SelectedCell[T]{Value: value, Present: present, Tag: owned})
	}
	return cells.rows, true
}

// Members is reusable storage for materializing one delivered span as the
// summary vector a fold reads.
type Members[T any] struct {
	rows []operand.MemberCell[T]
}

// NewMembers reserves storage for one caller's materializations.
func NewMembers[T any](reserve int) (*Members[T], bool) {
	if reserve < 0 {
		return nil, false
	}
	return &Members[T]{rows: make([]operand.MemberCell[T], 0, reserve)}, true
}

// Fill materializes the delivered span as the vector a fold reads.
//
// Position is the whole of a vector's meaning: a cell sits at the ordinal its
// owner declared it at, so an absent cell is carried as absent and never
// compacted away, which would renumber every later one.
func (members *Members[T]) Fill(span Span[T]) (operand.SummaryVector[T], bool) {
	if members == nil {
		return operand.SummaryVector[T]{}, false
	}
	members.rows = members.rows[:0]
	for index := 0; index < span.Len(); index++ {
		value, present, available := span.At(index)
		if !available {
			return operand.SummaryVector[T]{}, false
		}
		members.rows = append(members.rows, operand.MemberCell[T]{Value: value, Present: present})
	}
	return operand.NewMemberVector(members.rows)
}

// Rows returns the storage this materialization refills. It is how a caller
// that clones itself per worker sizes the clone at the same width.
func (cells *Cells[T]) Rows() []operand.SelectedCell[T] {
	if cells == nil {
		return nil
	}
	return cells.rows
}
