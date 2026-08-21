// Package lexical owns atomic Body publication, symbol visibility, and closures.
package lexical

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	modulelower "github.com/wippyai/go-lua/analysis/lua/lower/internal/module"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Bodies is the one atomic authority for lexical scopes and their Program roots.
// Its fields stay private so restoring symbol visibility and publishing a Body
// cannot be split across packages.
type Bodies struct {
	collector  *assembly.Collector
	binding    *bind.Result
	sourceName string
	modules    *modulelower.Writer
	phases     *continuation.Stack
	values     *eval.Values
	statics    *continuation.Statics
	localSteps []localStep

	active map[bind.Symbol]keyspace.Term
	undo   []activeUndo
	frames []bodyFrame
	source []sourceItem

	cellInline   [4]keyspace.Term
	cellOverflow []keyspace.Term
	cellLen      int
	chunkVararg  keyspace.Term
	reserved     []reservedCell
	reservedIDs  map[bind.Symbol]struct{}
	captures     []authored.Capture
}

// New creates the one lexical authority for one unfinished Program. It binds
// the sole phase stack once; copied lexical state or caller-supplied alternate
// stacks could split Cell visibility from Body publication and are forbidden.
func New(
	phases *continuation.Stack,
	collector *assembly.Collector,
	binding *bind.Result,
	sourceName string,
	modules *modulelower.Writer,
	values *eval.Values,
	statics *continuation.Statics,
) *Bodies {
	return &Bodies{
		collector:  collector,
		binding:    binding,
		sourceName: sourceName,
		modules:    modules,
		phases:     phases,
		values:     values,
		statics:    statics,
	}
}

// Clean reports whether every lexical transaction and scratch range completed.
func (b *Bodies) Clean() bool {
	return len(b.frames) == 0 &&
		len(b.source) == 0 &&
		len(b.active) == 0 &&
		len(b.undo) == 0 &&
		b.cellLen == 0 &&
		len(b.cellOverflow) == 0 &&
		len(b.reserved) == 0 &&
		len(b.reservedIDs) == 0 &&
		len(b.captures) == 0 &&
		len(b.localSteps) == 0
}

func (b *Bodies) span(holder ast.PositionHolder) source.Span {
	return coord.Span(b.sourceName, holder)
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
