package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) finishReturn(current step) error {
	values, _ := w.phases.Result()
	if current.ret == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: return continuation lost its host Body")
	}
	term, err := w.Return(current.span, current.host, values)
	if err != nil {
		return err
	}
	return w.appendTo(current.host, term)
}

func (w *Writer) finishIfCondition(current step) error {
	stmt := current.ifStmt
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid if continuation")
	}
	condition, _ := w.phases.Result()
	return w.openBody(stmt.Then, w.chunkSpan(stmt.Then), step{
		kind: finishIfThenStep, host: current.host, span: current.span,
		ifStmt: stmt, condition: condition,
	})
}

func (w *Writer) finishIfThen(current step) error {
	whenTrue, _ := w.phases.Result()
	if whenTrue == 0 || whenTrue != current.body {
		return fmt.Errorf("lualower: mismatched then Body")
	}
	stmt := current.ifStmt
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid then continuation")
	}
	return w.openBody(stmt.Else, w.chunkSpan(stmt.Else), step{
		kind: finishIfElseStep, host: current.host, span: current.span,
		ifStmt:    stmt,
		condition: current.condition, whenTrue: whenTrue,
	})
}

func (w *Writer) finishIfElse(current step) error {
	whenFalse, _ := w.phases.Result()
	if whenFalse == 0 || whenFalse != current.body {
		return fmt.Errorf("lualower: mismatched else Body")
	}
	if current.ifStmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: branch continuation lost its host Body")
	}
	term, err := w.Branch(current.span, current.host, current.condition, current.whenTrue, whenFalse)
	if err != nil {
		return err
	}
	return w.appendTo(current.host, term)
}

func (w *Writer) openBody(stmts []ast.Stmt, span source.Span, next step) error {
	if next.host == 0 || w.scopes.Owner() != next.host {
		return fmt.Errorf("lualower: branch Body opened outside its host Body")
	}
	body, err := w.scopes.EnterBlock(span)
	if err != nil {
		return fmt.Errorf("lualower: could not create branch Body: %w", err)
	}
	next.body = body
	return w.scheduleBody(stmts, span, next)
}
