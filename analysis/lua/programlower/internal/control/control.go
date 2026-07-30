// Package control owns authored control relations and their flow algebra.
package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/programlower/internal/lexical"
	"github.com/wippyai/go-lua/program"
)

// Writer is the one direct writer for authored terminal and structured control.
type Writer struct {
	builder *program.Builder

	cellInline   [4]program.Term
	cellOverflow []program.Term
	cellLen      int
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

// Return records a function exit and its terminal flow.
func (w *Writer) Return(
	span program.Span,
	owner program.Term,
	values program.Term,
) (program.Term, lexical.Flow, error) {
	term := w.builder.Return(span, owner, values)
	if term == 0 {
		return 0, lexical.Flow{}, fmt.Errorf("programlower: could not lower return")
	}
	return term, lexical.Flow{Terminated: true}, nil
}

// Break records a loop exit. Seal resolves its nearest same-function Loop.
func (w *Writer) Break(
	span program.Span,
	owner program.Term,
) (program.Term, lexical.Flow, error) {
	term := w.builder.Break(span, owner)
	if term == 0 {
		return 0, lexical.Flow{}, fmt.Errorf("programlower: could not lower break")
	}
	return term, lexical.Flow{Terminated: true, HasBreak: true}, nil
}

// Branch records one authored selection and composes both child flows.
func (w *Writer) Branch(
	span program.Span,
	owner program.Term,
	condition program.Term,
	whenTrue program.Term,
	whenFalse program.Term,
	thenFlow lexical.Flow,
	elseFlow lexical.Flow,
) (program.Term, lexical.Flow, error) {
	term := w.builder.Branch(span, owner, condition, whenTrue, whenFalse)
	if term == 0 {
		return 0, lexical.Flow{}, fmt.Errorf("programlower: could not create Branch")
	}
	return term, lexical.Flow{
		Terminated: thenFlow.Terminated && elseFlow.Terminated,
		HasBreak:   thenFlow.HasBreak || elseFlow.HasBreak,
	}, nil
}

// Loop publishes one structurally owned loop Body and consumes its pending
// per-iteration Cells. Every inner break is consumed; only a repeat Body with
// no fallthrough and no reachable break terminates its outer Body.
func (w *Writer) Loop(
	span program.Span,
	owner program.Term,
	body program.Term,
	control program.Term,
	cellMark int,
	kind program.LoopKind,
	bodyFlow lexical.Flow,
) (program.Term, lexical.Flow, error) {
	if cellMark < 0 || cellMark > w.cellLen {
		return 0, lexical.Flow{}, fmt.Errorf("programlower: invalid loop Cell mark")
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
		return 0, lexical.Flow{}, fmt.Errorf("programlower: could not lower loop")
	}
	var flow lexical.Flow
	if kind == program.LoopRepeat && bodyFlow.Terminated && !bodyFlow.HasBreak {
		flow.Terminated = true
	}
	return term, flow, nil
}

// Clean reports whether every pending loop-cell range completed.
func (w *Writer) Clean() bool {
	return w.cellLen == 0 && len(w.cellOverflow) == 0
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
