// Package control owns authored control relations.
package control

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program"
)

// Writer is the one direct writer for authored terminal and structured control.
type Writer struct {
	builder *program.Builder

	cellInline   [4]program.Term
	cellOverflow []program.Term
	cellLen      int

	labels     map[*ast.LabelStmt]labelState
	labelCount int
}

type labelState struct {
	term   program.Term
	placed bool
}

// New creates the control authority for one unfinished Program.
func New(builder *program.Builder) Writer {
	return Writer{builder: builder}
}

// CellMark identifies the start of one loop's pending per-iteration Cells.
func (w *Writer) CellMark() int {
	return w.cellLen
}

// RememberCell retains one loop Cell in declaration order.
func (w *Writer) RememberCell(cell program.Term) error {
	if cell == 0 {
		return fmt.Errorf("programlower: could not retain loop Cell")
	}
	w.appendCell(cell)
	return nil
}

// PredeclareLabel allocates one addressable source label before the Body is
// traversed, so both forward and outward Gotos can carry their final typed
// target without an unresolved Program relation.
func (w *Writer) PredeclareLabel(
	stmt *ast.LabelStmt,
	span program.Span,
	owner program.Term,
) error {
	if stmt == nil {
		return fmt.Errorf("programlower: cannot predeclare nil Label")
	}
	if w.labels == nil {
		w.labels = make(map[*ast.LabelStmt]labelState)
	}
	if _, exists := w.labels[stmt]; exists {
		return fmt.Errorf("programlower: duplicate Label predeclaration")
	}
	label := w.builder.Label(span, owner)
	if label == 0 {
		return fmt.Errorf("programlower: could not create Label")
	}
	w.labels[stmt] = labelState{term: label}
	return nil
}

// PlaceLabel fixes one predeclared Label at its exact gap between Body roots.
// Labels are void metadata and therefore do not advance the cursor.
func (w *Writer) PlaceLabel(stmt *ast.LabelStmt, cursor int) error {
	state, ok := w.labels[stmt]
	if !ok || state.term == 0 {
		return fmt.Errorf("programlower: Label was not predeclared")
	}
	if state.placed {
		return fmt.Errorf("programlower: Label was placed twice")
	}
	if !w.builder.SetLabelCursor(state.term, cursor) {
		return fmt.Errorf("programlower: could not place Label")
	}
	state.placed = true
	w.labels[stmt] = state
	w.labelCount++
	return nil
}

// Goto records one resolved non-local transfer. Label names and resolution
// remain transient binder concerns; Program receives only the exact Label.
func (w *Writer) Goto(
	span program.Span,
	owner program.Term,
	target *ast.LabelStmt,
) (program.Term, error) {
	state, ok := w.labels[target]
	if !ok || state.term == 0 {
		return 0, fmt.Errorf("programlower: Goto target was not predeclared")
	}
	term := w.builder.Goto(span, owner, state.term)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not lower goto")
	}
	return term, nil
}

// Return records a function exit.
func (w *Writer) Return(
	span program.Span,
	owner program.Term,
	values program.Term,
) (program.Term, error) {
	term := w.builder.Return(span, owner, values)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not lower return")
	}
	return term, nil
}

// Break records a loop exit. Seal resolves its nearest same-function Loop.
func (w *Writer) Break(
	span program.Span,
	owner program.Term,
) (program.Term, error) {
	term := w.builder.Break(span, owner)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not lower break")
	}
	return term, nil
}

// Branch records one authored selection.
func (w *Writer) Branch(
	span program.Span,
	owner program.Term,
	condition program.Term,
	whenTrue program.Term,
	whenFalse program.Term,
) (program.Term, error) {
	term := w.builder.Branch(span, owner, condition, whenTrue, whenFalse)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create Branch")
	}
	return term, nil
}

// Loop publishes one structurally owned loop Body and consumes its pending
// per-iteration Cells. Seal owns all exit and recurrence judgments.
func (w *Writer) Loop(
	span program.Span,
	owner program.Term,
	body program.Term,
	control program.Term,
	cellMark int,
	kind program.LoopKind,
) (program.Term, error) {
	if cellMark < 0 || cellMark > w.cellLen {
		return 0, fmt.Errorf("programlower: invalid loop Cell mark")
	}
	term := w.builder.Loop(
		span,
		owner,
		body,
		control,
		w.cellSlice()[cellMark:],
		kind,
	)
	w.truncateCells(cellMark)
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not lower loop")
	}
	return term, nil
}

// Clean reports whether every pending loop-cell range completed.
func (w *Writer) Clean() bool {
	return w.cellLen == 0 &&
		len(w.cellOverflow) == 0 &&
		len(w.labels) == w.labelCount
}

func (w *Writer) appendCell(cell program.Term) {
	if w.cellLen < len(w.cellInline) {
		w.cellInline[w.cellLen] = cell
		w.cellLen++
		return
	}
	if w.cellLen == len(w.cellInline) {
		w.cellOverflow = append(w.cellOverflow[:0], w.cellInline[:]...)
	}
	w.cellOverflow = append(w.cellOverflow, cell)
	w.cellLen++
}

func (w *Writer) cellSlice() []program.Term {
	if w.cellLen <= len(w.cellInline) {
		return w.cellInline[:w.cellLen]
	}
	return w.cellOverflow[:w.cellLen]
}

func (w *Writer) truncateCells(mark int) {
	w.cellLen = mark
	if mark <= len(w.cellInline) {
		w.cellOverflow = w.cellOverflow[:0]
		return
	}
	w.cellOverflow = w.cellOverflow[:mark]
}
