package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// CellMark identifies the start of one local or formal Cell group.
func (b *Bodies) CellMark() int {
	return b.cellLen
}

// CaptureMark identifies the start of one closure capture group.
func (b *Bodies) CaptureMark() int {
	return len(b.captures)
}

// Declare creates and installs one local, formal, or loop Cell.
func (b *Bodies) Declare(id bind.Symbol, span source.Span) (keyspace.Term, error) {
	return b.declare(id, span, true)
}

// DeclareLoop creates and installs one per-iteration Cell without retaining it
// in local/formal construction scratch.
func (b *Bodies) DeclareLoop(id bind.Symbol, span source.Span) (keyspace.Term, error) {
	return b.declare(id, span, false)
}

func (b *Bodies) declare(
	id bind.Symbol,
	span source.Span,
	retain bool,
) (keyspace.Term, error) {
	if id == 0 {
		return 0, fmt.Errorf("lualower: cannot declare zero binder symbol")
	}
	if _, exists := b.active[id]; exists {
		return 0, fmt.Errorf("lualower: duplicate active binder symbol")
	}
	owner := b.Owner()
	cell := b.collector.Cell(span, owner)
	if cell == 0 {
		return 0, fmt.Errorf("lualower: could not create Cell")
	}
	b.install(id, cell)
	if retain {
		b.appendCell(cell)
	}
	return cell, nil
}

// Capture creates and installs one inner Cell while retaining its exact outer.
func (b *Bodies) Capture(
	id bind.Symbol,
	span source.Span,
	outer keyspace.Term,
) (keyspace.Term, error) {
	owner := b.Owner()
	inner := b.collector.Cell(span, owner)
	if inner == 0 {
		return 0, fmt.Errorf("lualower: could not create capture Cell")
	}
	b.captures = append(b.captures, flow.Capture{Inner: inner, Outer: outer})
	b.install(id, inner)
	return inner, nil
}

func (b *Bodies) appendCell(cell keyspace.Term) {
	if b.cellLen < len(b.cellInline) {
		b.cellInline[b.cellLen] = cell
		b.cellLen++
		return
	}
	if b.cellLen == len(b.cellInline) {
		b.cellOverflow = append(b.cellOverflow[:0], b.cellInline[:]...)
	}
	b.cellOverflow = append(b.cellOverflow, cell)
	b.cellLen++
}

func (b *Bodies) cellSlice() []keyspace.Term {
	if b.cellLen <= len(b.cellInline) {
		return b.cellInline[:b.cellLen]
	}
	return b.cellOverflow[:b.cellLen]
}

func (b *Bodies) truncateCells(mark int) {
	b.cellLen = mark
	if mark <= len(b.cellInline) {
		b.cellOverflow = b.cellOverflow[:0]
		return
	}
	b.cellOverflow = b.cellOverflow[:mark]
}
