package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) scheduleBody(stmts []ast.Stmt, span source.Span, next step) error {
	// The body inbox retains the concrete child Body and resolved span. The
	// source runner owns the three body phases; control never recovers either
	// value from whichever lexical Body happens to be active later.
	w.push(next)
	if err := w.bodies.PushClose(next.body, span); err != nil {
		return err
	}
	if err := w.bodies.PushStatements(stmts, 0, next.body, span); err != nil {
		return err
	}
	return w.bodies.PushPrepare(stmts, next.body, span)
}

func (w *Writer) expression(expr ast.Expr, host keyspace.Term, span source.Span) error {
	if expr == nil || host == 0 || span.File == "" {
		return fmt.Errorf("lualower: absent control expression")
	}
	return w.expressions.Push(expr, host, span)
}

func (w *Writer) appendTo(host, term keyspace.Term) error {
	if host == 0 || term == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: control result has no active host Body")
	}
	return w.scopes.Append(term)
}

func (w *Writer) valueList(exprs []ast.Expr, host keyspace.Term, span source.Span) error {
	if host == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid control Values host")
	}
	return w.values.ScheduleValues(exprs, host, span)
}

func (w *Writer) push(next step) {
	w.steps = append(w.steps, next)
	w.phases.Push(continuation.Control)
}
