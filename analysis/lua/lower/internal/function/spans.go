package function

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
