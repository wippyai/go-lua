// Package lexical owns atomic Body publication, symbol visibility, and closures.
package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program"
)

// Bodies is the one atomic authority for lexical scopes and their Program roots.
// Its fields stay private so restoring symbol visibility and publishing a Body
// cannot be split across packages.
type Bodies struct {
	builder *program.Builder

	active map[symbol.ID]program.Term
	undo   []activeUndo
	frames []bodyFrame
	roots  []program.Term

	cellInline   [4]program.Term
	cellOverflow []program.Term
	cellLen      int
	reserved     []reservedCell
	reservedIDs  map[symbol.ID]struct{}
	captures     []program.Capture
}

type activeUndo struct {
	id      symbol.ID
	prior   program.Term
	existed bool
}

type reservedCell struct {
	id   symbol.ID
	cell program.Term
}

type bodyFrame struct {
	body     program.Term
	function *ast.FunctionExpr
	undoMark int
	rootMark int
}

// New creates the lexical authority for one unfinished Program.
func New(builder *program.Builder) Bodies {
	return Bodies{
		builder: builder,
		active:  make(map[symbol.ID]program.Term),
	}
}

// Entry creates, selects, and enters the canonical chunk Body.
func (b *Bodies) Entry(span program.Span) (program.Term, error) {
	body := b.builder.Body(span)
	if body == 0 {
		return 0, fmt.Errorf("programlower: could not create chunk body")
	}
	if !b.builder.SetEntry(body) {
		return 0, fmt.Errorf("programlower: could not set chunk Entry")
	}
	b.enter(body, nil)
	return body, nil
}

// EnterBlock creates one child Body that inherits its function boundary.
func (b *Bodies) EnterBlock(span program.Span) (program.Term, error) {
	function := b.frames[len(b.frames)-1].function
	return b.enterBody(span, function)
}

// EnterFunction creates one child Body at a new function boundary.
func (b *Bodies) EnterFunction(
	span program.Span,
	function *ast.FunctionExpr,
) (program.Term, error) {
	if function == nil {
		return 0, fmt.Errorf("programlower: nil function boundary")
	}
	return b.enterBody(span, function)
}

func (b *Bodies) enterBody(
	span program.Span,
	function *ast.FunctionExpr,
) (program.Term, error) {
	body := b.builder.Body(span)
	if body == 0 {
		return 0, fmt.Errorf("programlower: could not create Body")
	}
	b.enter(body, function)
	return body, nil
}

func (b *Bodies) enter(body program.Term, function *ast.FunctionExpr) {
	b.frames = append(b.frames, bodyFrame{
		body:     body,
		function: function,
		undoMark: len(b.undo),
		rootMark: len(b.roots),
	})
}

// Owner returns the active lexical Body.
func (b *Bodies) Owner() program.Term {
	return b.frames[len(b.frames)-1].body
}

// Cursor returns the active Body's structural position between authored roots.
// Labels at the same cursor share one source-control position and do not
// themselves consume a root.
func (b *Bodies) Cursor() int {
	frame := b.frames[len(b.frames)-1]
	return len(b.roots) - frame.rootMark
}

// Finish atomically publishes the current Body, restores its lexical mappings,
// and leaves its parent active.
func (b *Bodies) Finish() (program.Term, error) {
	if len(b.frames) == 0 {
		return 0, fmt.Errorf("programlower: no lexical body to finalize")
	}
	frame := b.frames[len(b.frames)-1]
	if !b.builder.SetBody(frame.body, b.roots[frame.rootMark:]...) {
		return 0, fmt.Errorf("programlower: could not finalize Body")
	}
	b.roots = b.roots[:frame.rootMark]
	b.restore(frame.undoMark)
	b.frames = b.frames[:len(b.frames)-1]
	return frame.body, nil
}

// Append publishes one completed statement relation. Reachability and
// completion are proved from the sealed control equations, never guessed by
// the source-order lowerer.
func (b *Bodies) Append(term program.Term) error {
	if term == 0 {
		return fmt.Errorf("programlower: could not create Body root")
	}
	b.roots = append(b.roots, term)
	return nil
}

// Resolve returns the currently visible Cell for one binder identity.
func (b *Bodies) Resolve(id symbol.ID) (program.Term, bool) {
	term, ok := b.active[id]
	return term, ok && term != 0
}

// Vararg resolves the active function boundary's exact vararg Cell.
func (b *Bodies) Vararg(binding *bind.Result) (program.Term, error) {
	function := b.frames[len(b.frames)-1].function
	if function == nil {
		return 0, fmt.Errorf("programlower: vararg expression outside function")
	}
	id, ok := binding.VarargSymbol(function)
	if !ok {
		return 0, fmt.Errorf("programlower: vararg expression in non-vararg function")
	}
	cell, ok := b.Resolve(id)
	if !ok {
		return 0, fmt.Errorf("programlower: missing vararg Cell")
	}
	return cell, nil
}

// Has reports whether a binder identity already has an active Cell.
func (b *Bodies) Has(id symbol.ID) bool {
	_, ok := b.active[id]
	return ok
}

// CellMark identifies the start of one local or formal Cell group.
func (b *Bodies) CellMark() int {
	return b.cellLen
}

// ReservedMark starts one uninstalled local-declaration transaction. Reserved
// Cells can host static declared types, but are intentionally invisible to
// initializer lowering until BindReserved atomically publishes them.
func (b *Bodies) ReservedMark() int { return len(b.reserved) }

// CaptureMark identifies the start of one closure capture group.
func (b *Bodies) CaptureMark() int {
	return len(b.captures)
}

// Declare creates and installs one local, formal, or loop Cell.
func (b *Bodies) Declare(id symbol.ID, span program.Span) (program.Term, error) {
	return b.declare(id, span, true)
}

// Reserve creates one local Cell without installing its binder identity.
func (b *Bodies) Reserve(id symbol.ID, span program.Span) (program.Term, error) {
	if id == 0 {
		return 0, fmt.Errorf("programlower: cannot reserve zero binder symbol")
	}
	if _, exists := b.active[id]; exists {
		return 0, fmt.Errorf("programlower: duplicate active binder symbol")
	}
	if _, exists := b.reservedIDs[id]; exists {
		return 0, fmt.Errorf("programlower: duplicate reserved binder symbol")
	}
	cell := b.builder.Cell(span, b.Owner())
	if cell == 0 {
		return 0, fmt.Errorf("programlower: could not reserve Cell")
	}
	b.appendCell(cell)
	b.reserved = append(b.reserved, reservedCell{id: id, cell: cell})
	if b.reservedIDs == nil {
		b.reservedIDs = make(map[symbol.ID]struct{})
	}
	b.reservedIDs[id] = struct{}{}
	return cell, nil
}

// DeclareLoop creates and installs one per-iteration Cell without retaining it
// in local/formal construction scratch.
func (b *Bodies) DeclareLoop(id symbol.ID, span program.Span) (program.Term, error) {
	return b.declare(id, span, false)
}

func (b *Bodies) declare(
	id symbol.ID,
	span program.Span,
	retain bool,
) (program.Term, error) {
	if id == 0 {
		return 0, fmt.Errorf("programlower: cannot declare zero binder symbol")
	}
	if _, exists := b.active[id]; exists {
		return 0, fmt.Errorf("programlower: duplicate active binder symbol")
	}
	owner := b.Owner()
	cell := b.builder.Cell(span, owner)
	if cell == 0 {
		return 0, fmt.Errorf("programlower: could not create Cell")
	}
	b.install(id, cell)
	if retain {
		b.appendCell(cell)
	}
	return cell, nil
}

// Bind completes one local declaration group and publishes it as a root.
func (b *Bodies) Bind(mark int, span program.Span, values program.Term) error {
	if mark < 0 || mark > b.cellLen {
		return fmt.Errorf("programlower: invalid local Cell mark")
	}
	owner := b.Owner()
	term := b.builder.Bind(span, owner, b.cellSlice()[mark:], values)
	b.truncateCells(mark)
	if term == 0 {
		return fmt.Errorf("programlower: could not lower local declaration")
	}
	b.roots = append(b.roots, term)
	return nil
}

// ReservedCell returns one Cell in an unfinished local-declaration transaction.
func (b *Bodies) ReservedCell(mark, index int) (program.Term, bool) {
	at := mark + index
	if mark < 0 || index < 0 || at < mark || at >= len(b.reserved) {
		return 0, false
	}
	return b.reserved[at].cell, true
}

// BindReserved publishes one reserved local group after its initializer and
// declared types were lowered. Installation happens only after Builder.Bind
// succeeds, preserving Lua's initializer visibility boundary.
func (b *Bodies) BindReserved(cellMark, reservedMark int, span program.Span, values program.Term) error {
	if cellMark < 0 || cellMark > b.cellLen || reservedMark < 0 || reservedMark > len(b.reserved) {
		return fmt.Errorf("programlower: invalid reserved local mark")
	}
	cells := b.cellSlice()[cellMark:]
	reserved := b.reserved[reservedMark:]
	if len(cells) != len(reserved) {
		return fmt.Errorf("programlower: reserved local Cell range mismatch")
	}
	for index, item := range reserved {
		if item.id == 0 || item.cell != cells[index] || b.Has(item.id) {
			return fmt.Errorf("programlower: invalid reserved local Cell")
		}
		if _, exists := b.reservedIDs[item.id]; !exists {
			return fmt.Errorf("programlower: missing reserved binder symbol")
		}
	}
	owner := b.Owner()
	term := b.builder.Bind(span, owner, cells, values)
	if term == 0 {
		return fmt.Errorf("programlower: could not lower reserved local declaration")
	}
	for _, item := range reserved {
		b.install(item.id, item.cell)
		delete(b.reservedIDs, item.id)
	}
	b.truncateCells(cellMark)
	b.reserved = b.reserved[:reservedMark]
	b.roots = append(b.roots, term)
	return nil
}

// Capture creates and installs one inner Cell while retaining its exact outer.
func (b *Bodies) Capture(
	id symbol.ID,
	span program.Span,
	outer program.Term,
) (program.Term, error) {
	owner := b.Owner()
	inner := b.builder.Cell(span, owner)
	if inner == 0 {
		return 0, fmt.Errorf("programlower: could not create capture Cell")
	}
	b.captures = append(b.captures, program.Capture{Inner: inner, Outer: outer})
	b.install(id, inner)
	return inner, nil
}

// DeclareFunction mints the closure identity before entering its child Body so
// generic constraints can retain the outer lexical frontier.
func (b *Bodies) DeclareFunction(span program.Span) (program.Term, error) {
	if len(b.frames) == 0 {
		return 0, fmt.Errorf("programlower: Function has no lexical owner")
	}
	term := b.builder.DeclareFunction(span, b.Owner())
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not declare Function")
	}
	if !b.builder.SetFunctionOuterGap(term, b.Cursor()) {
		return 0, fmt.Errorf("programlower: could not place Function")
	}
	return term, nil
}

// FillFunction completes a previously declared closure from cells and
// captures installed by the machine's binder-ordered traversal.
func (b *Bodies) FillFunction(
	function program.Term,
	cellMark int,
	captureMark int,
	varargIndex int,
) error {
	if len(b.frames) < 2 {
		return fmt.Errorf("programlower: Function has no lexical owner")
	}
	if cellMark < 0 || cellMark > b.cellLen ||
		captureMark < 0 || captureMark > len(b.captures) {
		return fmt.Errorf("programlower: invalid Function construction mark")
	}
	params := b.cellSlice()[cellMark:]
	formals := params
	var vararg program.Term
	if varargIndex >= 0 {
		if varargIndex >= len(params) {
			return fmt.Errorf("programlower: invalid function vararg Cell")
		}
		if varargIndex != len(params)-1 {
			return fmt.Errorf("programlower: function vararg Cell is not final")
		}
		vararg = params[varargIndex]
		formals = params[:varargIndex]
	}
	body := b.frames[len(b.frames)-1].body
	if !b.builder.FillFunction(function, body, formals, vararg, b.captures[captureMark:]) {
		return fmt.Errorf("programlower: could not fill Function")
	}
	b.truncateCells(cellMark)
	b.captures = b.captures[:captureMark]
	return nil
}

func (b *Bodies) install(id symbol.ID, term program.Term) {
	prior, existed := b.active[id]
	b.undo = append(b.undo, activeUndo{id: id, prior: prior, existed: existed})
	b.active[id] = term
}

func (b *Bodies) appendCell(cell program.Term) {
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

func (b *Bodies) cellSlice() []program.Term {
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

func (b *Bodies) restore(mark int) {
	for i := len(b.undo) - 1; i >= mark; i-- {
		undo := b.undo[i]
		if undo.existed {
			b.active[undo.id] = undo.prior
		} else {
			delete(b.active, undo.id)
		}
	}
	b.undo = b.undo[:mark]
}

// Clean reports whether every lexical transaction and scratch range completed.
func (b *Bodies) Clean() bool {
	return len(b.frames) == 0 &&
		len(b.roots) == 0 &&
		len(b.active) == 0 &&
		len(b.undo) == 0 &&
		b.cellLen == 0 &&
		len(b.cellOverflow) == 0 &&
		len(b.reserved) == 0 &&
		len(b.reservedIDs) == 0 &&
		len(b.captures) == 0
}
