// Package function owns executable Function construction during Program
// lowering. It keeps function-origin validation, closure identity, lexical
// entry, formal/capture construction, and authored static headers together.
// Source dispatches only the closed owner tokens emitted here.
package function

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	staticlower "github.com/wippyai/go-lua/analysis/lua/lower/internal/static"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/storage"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Writer is the one executable-function authority for an unfinished Program.
// It owns no source-wide scheduler or alternate result channel: continuation.Stack is
// the sole continuation/result crossing with source.
type Writer struct {
	stack       *continuation.Stack
	collector   *assembly.Collector
	binding     *bind.Result
	scopes      *lexical.Bodies
	packs       *eval.Values
	access      *storage.Writer
	static      *staticlower.Writer
	expressions *continuation.Expressions
	bodies      *continuation.Bodies
	statics     *continuation.Statics
	sourceName  string
	captures    map[*ast.FunctionExpr][]bind.Capture
	steps       []step
}

// New creates the executable-function authority. Capture entries are indexed
// once from the binder's boundary stream; no caller-owned capture projection
// is retained or reconstructed per Function.
func New(
	stack *continuation.Stack,
	collector *assembly.Collector,
	binding *bind.Result,
	scopes *lexical.Bodies,
	packs *eval.Values,
	access *storage.Writer,
	static *staticlower.Writer,
	expressions *continuation.Expressions,
	bodies *continuation.Bodies,
	statics *continuation.Statics,
	sourceName string,
) Writer {
	w := Writer{
		stack: stack, collector: collector, binding: binding, scopes: scopes, packs: packs,
		access: access, static: static, expressions: expressions, bodies: bodies,
		statics: statics, sourceName: sourceName,
	}
	if binding != nil {
		binding.ForEachEntryCapture(func(fn *ast.FunctionExpr, capture bind.Capture) bool {
			if w.captures == nil {
				w.captures = make(map[*ast.FunctionExpr][]bind.Capture)
			}
			w.captures[fn] = append(w.captures[fn], capture)
			return true
		})
	}
	return w
}

// Clean reports that no executable-function continuation remains private.
func (w *Writer) Clean() bool { return w != nil && len(w.steps) == 0 }
