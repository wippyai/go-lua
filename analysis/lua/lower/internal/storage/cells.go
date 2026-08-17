package storage

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Global selects the one Program-scoped Cell for a binder-authorized identity.
func (w *Writer) Global(identity bind.GlobalIdentity) (keyspace.Term, error) {
	if w == nil || w.collector == nil || !identity.Valid() {
		return 0, fmt.Errorf("lualower: missing storage authority")
	}
	term := w.collector.Global(identity)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not select global Cell")
	}
	return term, nil
}

// Read records an ordinary observation of an already selected Cell or Lens.
// Implicit-global evidence is selected only by ScheduleExpression from binder
// evidence and Static's owned depth; callers never supply that policy.
func (w *Writer) Read(span source.Span, owner, source keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("lualower: missing storage authority")
	}
	term := w.collector.Read(span, owner, source)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not read storage")
	}
	return term, nil
}

func (w *Writer) implicitRead(span source.Span, owner, global keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.collector == nil {
		return 0, fmt.Errorf("lualower: missing storage authority")
	}
	term := w.collector.ImplicitRead(span, owner, global)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not read implicit global Cell")
	}
	return term, nil
}

// ResolveCell selects an identifier's exact Cell without evaluating it. It is
// for source-only consumers that need storage identity rather than a Read.
func (w *Writer) ResolveCell(expr *ast.IdentExpr) (keyspace.Term, error) {
	if w == nil || w.binding == nil || w.lexical == nil || expr == nil {
		return 0, fmt.Errorf("lualower: invalid identifier storage selection")
	}
	id, ok := w.binding.SymbolOf(expr)
	if !ok || id == 0 {
		return 0, fmt.Errorf("lualower: binder has no symbol for identifier occurrence")
	}
	if cell, visible := w.lexical.Resolve(id); visible {
		return cell, nil
	}
	identity, global := w.binding.GlobalIdentity(expr)
	if !global {
		return 0, fmt.Errorf("lualower: unsupported non-local identifier binding")
	}
	return w.Global(identity)
}
