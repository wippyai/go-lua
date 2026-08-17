package control

import (
	"fmt"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Return records a function exit.
func (w *Writer) Return(
	span source.Span,
	owner keyspace.Term,
	values keyspace.Term,
) (keyspace.Term, error) {
	term := w.flow.Return(span, owner, values)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower return")
	}
	return term, nil
}

// Break records a loop exit. Seal resolves its nearest same-function Loop.
func (w *Writer) Break(
	span source.Span,
	owner keyspace.Term,
) (keyspace.Term, error) {
	term := w.flow.Break(span, owner)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower break")
	}
	return term, nil
}

// Branch records one authored selection.
func (w *Writer) Branch(
	span source.Span,
	owner keyspace.Term,
	condition keyspace.Term,
	whenTrue keyspace.Term,
	whenFalse keyspace.Term,
) (keyspace.Term, error) {
	term := w.flow.Branch(span, owner, condition, whenTrue, whenFalse)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not create Branch")
	}
	return term, nil
}

// Loop publishes one structurally owned loop Body and consumes its pending
// per-iteration Cells. Seal owns all exit and recurrence judgments.
func (w *Writer) Loop(
	span source.Span,
	owner keyspace.Term,
	body keyspace.Term,
	control keyspace.Term,
	cellMark int,
	loopKind flowkind.LoopKind,
) (keyspace.Term, error) {
	if cellMark < 0 || cellMark > w.cellLen {
		return 0, fmt.Errorf("lualower: invalid loop Cell mark")
	}
	term := w.flow.Loop(
		span,
		owner,
		body,
		control,
		w.cellSlice()[cellMark:],
		loopKind,
	)
	w.truncateCells(cellMark)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower loop")
	}
	return term, nil
}
