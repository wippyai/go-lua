package control

import (
	"fmt"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) finishWhileCondition(current step) error {
	stmt := current.while
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid while continuation")
	}
	control, _ := w.phases.Result()
	return w.openLoopBody(stmt.Stmts, w.chunkSpan(stmt.Stmts), step{
		kind: finishLoopStep, host: current.host, span: current.span, while: stmt,
		control: control, cellMark: w.CellMark(),
	})
}

func (w *Writer) beginRepeat(stmt *ast.RepeatStmt, host keyspace.Term, span source.Span) error {
	if stmt == nil || stmt.Condition == nil {
		return fmt.Errorf("lualower: invalid repeat statement")
	}
	if host == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: repeat began outside its host Body")
	}
	// The condition is evaluated after its body statements but before that body
	// closes, preserving repeat-until visibility of body-local Cells.
	body, err := w.scopes.EnterBlock(w.chunkSpan(stmt.Stmts))
	if err != nil {
		return fmt.Errorf("lualower: could not create repeat Body: %w", err)
	}
	w.push(step{
		kind: finishRepeatConditionStep, host: body, parent: host, span: span,
		repeat: stmt, body: body, cellMark: w.CellMark(),
	})
	bodySpan := w.chunkSpan(stmt.Stmts)
	if err := w.bodies.PushStatements(stmt.Stmts, 0, body, bodySpan); err != nil {
		return err
	}
	return w.bodies.PushPrepare(stmt.Stmts, body, bodySpan)
}

func (w *Writer) finishRepeatCondition(current step) error {
	stmt := current.repeat
	if stmt == nil || stmt.Condition == nil || current.body != current.host || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid repeat continuation")
	}
	w.push(step{
		kind: finishRepeatControlStep, host: current.host, parent: current.parent,
		span: current.span, repeat: stmt, body: current.body, cellMark: current.cellMark,
	})
	return w.expression(stmt.Condition, current.host, w.span(stmt.Condition))
}

func (w *Writer) finishRepeatControl(current step) error {
	stmt := current.repeat
	if stmt == nil || current.body != current.host || current.parent == 0 || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid repeat close continuation")
	}
	control, _ := w.phases.Result()
	w.push(step{
		kind: finishLoopStep, host: current.parent, span: current.span,
		repeat: stmt, body: current.body, control: control, cellMark: current.cellMark,
	})
	return w.bodies.PushClose(current.body, w.chunkSpan(stmt.Stmts))
}

func (w *Writer) beginNumberFor(stmt *ast.NumberForStmt, host keyspace.Term, span source.Span) error {
	if stmt == nil || stmt.Init == nil || stmt.Limit == nil {
		return fmt.Errorf("lualower: invalid numeric for statement")
	}
	exprs := []ast.Expr{stmt.Init, stmt.Limit}
	if stmt.Step != nil {
		exprs = append(exprs, stmt.Step)
	}
	if host == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: numeric for began outside its host Body")
	}
	w.push(step{kind: numberControlStep, host: host, span: span, number: stmt, exprs: exprs})
	return nil
}

func (w *Writer) runNumberControls(current step) error {
	stmt := current.number
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid numeric for continuation")
	}
	if current.index == len(current.exprs) {
		control, err := w.values.Fixed(current.span, current.host, current.terms)
		if err != nil {
			return err
		}
		return w.openLoopBody(stmt.Stmts, w.chunkSpan(stmt.Stmts), step{
			kind: finishLoopStep, host: current.host, span: current.span, number: stmt,
			control: control, cellMark: w.CellMark(),
		})
	}
	if current.index < 0 || current.index > len(current.exprs) {
		return fmt.Errorf("lualower: invalid numeric for cursor")
	}
	next := current
	next.index++
	w.push(step{kind: appendNumberControlStep, host: current.host, span: current.span, number: stmt, exprs: next.exprs, terms: next.terms, index: next.index})
	expr := current.exprs[current.index]
	return w.expression(expr, current.host, w.span(expr))
}

func (w *Writer) appendNumberControl(current step) error {
	term, _ := w.phases.Result()
	if term == 0 {
		return fmt.Errorf("lualower: absent numeric for control")
	}
	current.terms = append(current.terms, term)
	if current.number == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: numeric control continuation lost its host Body")
	}
	w.push(step{kind: numberControlStep, host: current.host, span: current.span, number: current.number, exprs: current.exprs, terms: current.terms, index: current.index})
	return nil
}

func (w *Writer) beginGenericFor(stmt *ast.GenericForStmt, host keyspace.Term, span source.Span) error {
	if stmt == nil || len(stmt.Names) == 0 || len(stmt.Exprs) == 0 {
		return fmt.Errorf("lualower: invalid generic for statement")
	}
	if host == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: generic for began outside its host Body")
	}
	w.push(step{kind: finishGenericControlsStep, host: host, span: span, generic: stmt})
	return w.valueList(stmt.Exprs, host, span)
}

func (w *Writer) finishGenericControls(current step) error {
	stmt := current.generic
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid generic for continuation")
	}
	control, _ := w.phases.Result()
	return w.openLoopBody(stmt.Stmts, w.chunkSpan(stmt.Stmts), step{
		kind: finishLoopStep, host: current.host, span: current.span, generic: stmt,
		control: control, cellMark: w.CellMark(),
	})
}

func (w *Writer) openLoopBody(stmts []ast.Stmt, span source.Span, next step) error {
	if next.host == 0 || w.scopes.Owner() != next.host {
		return fmt.Errorf("lualower: loop body opened outside its host Body")
	}
	body, err := w.scopes.EnterBlock(span)
	if err != nil {
		return fmt.Errorf("lualower: could not create loop Body: %w", err)
	}
	next.body = body
	if err := w.declareLoopCells(next); err != nil {
		return err
	}
	return w.scheduleBody(stmts, span, next)
}

func (w *Writer) declareLoopCells(next step) error {
	if next.body == 0 || w.scopes.Owner() != next.body {
		return fmt.Errorf("lualower: loop Cells declared outside their Body")
	}
	switch {
	case next.while != nil, next.repeat != nil:
		return nil
	case next.number != nil:
		loop := next.number
		id, ok := w.binding.NumForSymbol(loop)
		if !ok || id == 0 {
			return fmt.Errorf("lualower: binder has no numeric for symbol")
		}
		cell, err := w.scopes.DeclareLoop(id, w.positionSpan(loop.NamePosition))
		if err != nil {
			return err
		}
		return w.RememberCell(cell)
	case next.generic != nil:
		loop := next.generic
		ids := w.binding.GenericForSymbols(loop)
		if len(ids) != len(loop.Names) {
			return fmt.Errorf("lualower: binder has incomplete generic for symbols")
		}
		for index, id := range ids {
			if id == 0 {
				return fmt.Errorf("lualower: binder has zero generic for symbol %d", index)
			}
			span := w.span(loop)
			if index < len(loop.NamePositions) {
				span = w.positionSpan(loop.NamePositions[index])
			}
			cell, err := w.scopes.DeclareLoop(id, span)
			if err != nil {
				return err
			}
			if err := w.RememberCell(cell); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("lualower: loop continuation has no declaration form")
	}
	return nil
}

func (w *Writer) finishLoop(current step) error {
	body, _ := w.phases.Result()
	if body == 0 || body != current.body || current.host == 0 || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: mismatched loop Body")
	}
	var kind flowkind.LoopKind
	switch {
	case current.while != nil:
		kind = flowkind.LoopWhile
	case current.repeat != nil:
		kind = flowkind.LoopRepeat
	case current.number != nil:
		kind = flowkind.LoopNumericFor
	case current.generic != nil:
		kind = flowkind.LoopGenericFor
	default:
		return fmt.Errorf("lualower: loop continuation has no form")
	}
	term, err := w.Loop(current.span, current.host, body, current.control, current.cellMark, kind)
	if err != nil {
		return err
	}
	return w.appendTo(current.host, term)
}
