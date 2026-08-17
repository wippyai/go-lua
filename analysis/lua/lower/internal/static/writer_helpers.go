package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) term(term keyspace.Term, what string) (keyspace.Term, error) {
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not create %s", what)
	}
	return term, nil
}

func (w *Writer) rangeTerms(mark, count int) ([]keyspace.Term, error) {
	if w == nil || mark < 0 || count < 0 || mark > len(w.children) || len(w.children)-mark != count {
		return nil, fmt.Errorf("lualower: incomplete static type children")
	}
	terms := w.children[mark:]
	w.children = w.children[:mark]
	return terms, nil
}

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

func (w *Writer) nameSpan(position ast.Position) source.Span {
	file := position.File
	if file == "" {
		file = w.sourceName
	}
	span, ok := coord.Build(file, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(file)
	}
	return span
}
