package lexical

import (
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *Bodies) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: b.sourceName}
	}
	span, ok := coord.Build(b.sourceName, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return coord.Invalid(b.sourceName)
	}
	return span
}

func (b *Bodies) nameSpan(stmt *ast.LocalAssignStmt, index int) source.Span {
	if stmt == nil || index < 0 || index >= len(stmt.NamePositions) {
		return b.span(stmt)
	}
	position := stmt.NamePositions[index]
	if !position.Valid() {
		if position.Line == 0 && position.Column == 0 && position.EndLine == 0 && position.EndColumn == 0 {
			return source.Span{File: b.sourceName}
		}
		return coord.Invalid(b.sourceName)
	}
	span, ok := coord.Build(b.sourceName, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(b.sourceName)
	}
	return span
}
