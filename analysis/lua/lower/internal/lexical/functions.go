package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// DeclareFunction mints the closure identity before entering its child Body.
// Seal later derives the generic-constraint frontier from its exact source
// occurrence; the lowerer supplies no placement number.
func (b *Bodies) DeclareFunction(span source.Span) (keyspace.Term, error) {
	if len(b.frames) == 0 {
		return 0, fmt.Errorf("lualower: Function has no lexical owner")
	}
	term := b.collector.DeclareFunction(span, b.Owner())
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not declare Function")
	}
	return term, nil
}

// FillFunction completes a previously declared closure from cells and
// captures installed by the machine's binder-ordered traversal.
func (b *Bodies) FillFunction(
	function keyspace.Term,
	cellMark int,
	captureMark int,
	varargIndex int,
) error {
	if len(b.frames) < 2 {
		return fmt.Errorf("lualower: Function has no lexical owner")
	}
	if cellMark < 0 || cellMark > b.cellLen ||
		captureMark < 0 || captureMark > len(b.captures) {
		return fmt.Errorf("lualower: invalid Function construction mark")
	}
	params := b.cellSlice()[cellMark:]
	formals := params
	var vararg keyspace.Term
	if varargIndex >= 0 {
		if varargIndex >= len(params) {
			return fmt.Errorf("lualower: invalid function vararg Cell")
		}
		if varargIndex != len(params)-1 {
			return fmt.Errorf("lualower: function vararg Cell is not final")
		}
		vararg = params[varargIndex]
		formals = params[:varargIndex]
	}
	body := b.frames[len(b.frames)-1].body
	if !b.collector.FillFunction(function, body, formals, vararg, b.captures[captureMark:]) {
		return fmt.Errorf("lualower: could not fill Function")
	}
	b.truncateCells(cellMark)
	b.captures = b.captures[:captureMark]
	return nil
}
