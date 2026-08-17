package control

import (
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

func (w *Writer) chunkSpan(stmts []ast.Stmt) source.Span {
	if len(stmts) == 0 {
		return source.Span{File: w.sourceName}
	}
	first, last := stmts[0], stmts[len(stmts)-1]
	if first == nil || last == nil {
		return coord.Invalid(w.sourceName)
	}
	span, ok := coord.Build(w.sourceName, first.Line(), first.Column(), last.LastLine(), last.LastColumn())
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}
