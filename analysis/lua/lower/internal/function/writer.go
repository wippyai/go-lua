// Package function owns executable Function construction during Program
// lowering. It keeps function-origin validation, closure identity, lexical
// entry, formal/capture construction, and authored static headers together.
// Source dispatches only the closed owner tokens emitted here.
package function

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	staticlower "github.com/wippyai/go-lua/analysis/lua/lower/internal/static"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/storage"
	"github.com/wippyai/go-lua/analysis/program/source"
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

func (w *Writer) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: w.sourceName}
	}
	span, ok := coord.Build(w.sourceName, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}

func (w *Writer) nameSpan(stmt *ast.LocalAssignStmt, index int) source.Span {
	if stmt != nil && index >= 0 && index < len(stmt.NamePositions) {
		return w.positionSpan(stmt.NamePositions[index])
	}
	return w.span(stmt)
}

func (w *Writer) positionSpan(position ast.Position) source.Span {
	if !position.Valid() {
		if position.Line == 0 && position.Column == 0 && position.EndLine == 0 && position.EndColumn == 0 {
			return source.Span{File: w.sourceName}
		}
		return coord.Invalid(w.sourceName)
	}
	span, ok := coord.Build(w.sourceName, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}

func (w *Writer) methodSelectorSpan(receiver ast.Expr, position ast.Position) source.Span {
	span := w.positionSpan(position)
	if receiver == nil {
		return span
	}
	receiverSpan, ok := coord.Build(span.File, receiver.Line(), receiver.Column(), receiver.LastLine(), receiver.LastColumn())
	if !ok {
		return coord.Invalid(span.File)
	}
	if span.StartLine != receiverSpan.StartLine {
		return span
	}
	startCol := position.Column
	if receiverSpan.EndCol != 0 {
		if receiverSpan.EndCol == ^uint32(0) {
			return coord.Invalid(span.File)
		}
		startCol = int(receiverSpan.EndCol) + 1
	}
	if startCol <= 0 {
		startCol = position.Column
	}
	selector, ok := coord.Build(span.File, int(span.StartLine), startCol, int(span.EndLine), int(span.EndCol))
	if !ok {
		return coord.Invalid(span.File)
	}
	return selector
}
