// Package module owns direct module-boundary evidence during source lowering.
package module

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower/internal/collector"
	"github.com/wippyai/go-lua/program/source"
)

// Census is the typed source-order allocation input for Module Import rows.
// Binder owns the one complete syntax traversal; this layer only selects the
// already-bound direct global-call evidence for the named module entrypoint.
type Census struct {
	ordinal map[*ast.FuncCallExpr]int
}

// BuildCensus assigns final dense slots to statically resolvable imports. The
// binder has already proved that the call head is the unshadowed global
// require; module additionally requires the dialect's one non-empty authored
// string argument. Every other call remains ordinary call evidence.
//
// BuildCensus neither walks AST nor decides whether the admitted occurrence is
// in executable or static-query containment.
func BuildCensus(binding *bind.Result) (Census, error) {
	if binding == nil {
		return Census{}, fmt.Errorf("programlower: nil binding for module census")
	}
	ordinal := make(map[*ast.FuncCallExpr]int)
	for _, occurrence := range binding.DirectGlobalCalls() {
		if occurrence.Call == nil {
			return Census{}, fmt.Errorf("programlower: malformed direct global call evidence")
		}
		if !occurrence.Global.Matches("require") {
			continue
		}
		if len(occurrence.Call.Args) != 1 {
			continue
		}
		request, authored := occurrence.Call.Args[0].(*ast.StringExpr)
		if !authored || request == nil || request.Value == "" {
			continue
		}
		if _, duplicate := ordinal[occurrence.Call]; duplicate {
			return Census{}, fmt.Errorf("programlower: duplicate direct require evidence")
		}
		ordinal[occurrence.Call] = len(ordinal)
	}
	return Census{ordinal: ordinal}, nil
}

// Count reports the fixed final Import cardinality.
func (c Census) Count() int { return len(c.ordinal) }

// Ordinal returns one direct require's zero-based final dense slot.
func (c Census) Ordinal(call *ast.FuncCallExpr) (int, bool) {
	ordinal, ok := c.ordinal[call]
	return ordinal, ok
}

// Writer is the one direct-require and import-alias authority for an
// unfinished Program.
type Writer struct {
	module  collector.ModuleRoot
	census  Census
	imports map[*ast.FuncCallExpr]keyspace.Term
	filled  []bool
}

// New binds the allocation census to the collector's Module capability. It
// neither owns nor resolves lexical cells.
func New(module collector.ModuleRoot, census Census) Writer {
	return Writer{module: module, census: census, filled: make([]bool, census.Count())}
}

// ObserveCall records only the parser's plain call form and the binder's exact
// global require identity. Shadowed or aliased spellings remain ordinary Calls.
func (w *Writer) ObserveCall(call *ast.FuncCallExpr, span source.Span, term keyspace.Term) error {
	if w == nil || call == nil || term == 0 {
		return fmt.Errorf("programlower: invalid module Call observation")
	}
	ordinal, planned := w.census.Ordinal(call)
	if !planned {
		return nil
	}
	if w.imports == nil {
		w.imports = make(map[*ast.FuncCallExpr]keyspace.Term)
	}
	if ordinal < 0 || ordinal >= len(w.filled) || w.filled[ordinal] {
		return fmt.Errorf("programlower: direct require has no unique module census slot")
	}
	importTerm := w.module.Import(ordinal, span, term)
	if importTerm == 0 {
		return fmt.Errorf("programlower: could not create Import")
	}
	w.filled[ordinal] = true
	w.imports[call] = importTerm
	return nil
}

// AttachAlias retains only `local M = require(...)`'s one-cell binding. The
// lexical owner resolves alias before this call; module never reaches into
// lexical state. General aliases and assignments remain ordinary source
// semantics.
func (w *Writer) AttachAlias(stmt *ast.LocalAssignStmt, alias keyspace.Term) error {
	if w == nil {
		return fmt.Errorf("programlower: missing module authority")
	}
	if stmt == nil || len(stmt.Names) != 1 || len(stmt.Exprs) != 1 {
		return nil
	}
	call, ok := stmt.Exprs[0].(*ast.FuncCallExpr)
	if !ok || call == nil {
		return nil
	}
	importTerm := w.imports[call]
	if importTerm == 0 {
		return nil
	}
	if alias == 0 {
		return fmt.Errorf("programlower: Import alias Cell is absent")
	}
	if !w.module.SetImportAlias(importTerm, alias) {
		return fmt.Errorf("programlower: could not set Import alias")
	}
	return nil
}

// Clean reports whether no module evidence remains unresolved.
func (w *Writer) Clean() bool {
	if w == nil {
		return false
	}
	for call, importTerm := range w.imports {
		if call == nil || importTerm == 0 {
			return false
		}
	}
	for _, filled := range w.filled {
		if !filled {
			return false
		}
	}
	return true
}
