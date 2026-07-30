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
	captures     []program.Capture
}

type activeUndo struct {
	id      symbol.ID
	prior   program.Term
	existed bool
}

type bodyFrame struct {
	body       program.Term
	function   *ast.FunctionExpr
	span       program.Span
	undoMark   int
	rootMark   int
	terminated bool
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
	b.enter(body, nil, span)
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
	b.enter(body, function, span)
	return body, nil
}

func (b *Bodies) enter(body program.Term, function *ast.FunctionExpr, span program.Span) {
	b.frames = append(b.frames, bodyFrame{
		body:     body,
		function: function,
		span:     span,
		undoMark: len(b.undo),
		rootMark: len(b.roots),
	})
}

// Owner returns the active lexical Body.
func (b *Bodies) Owner() program.Term {
	return b.frames[len(b.frames)-1].body
}

// CanContinue reports whether the active Body still accepts a statement root.
func (b *Bodies) CanContinue() bool {
	return !b.frames[len(b.frames)-1].terminated
}

// Finish atomically publishes the current Body, restores its lexical mappings,
// and leaves its parent active.
func (b *Bodies) Finish(normalValues program.Term) (program.Term, bool, error) {
	if len(b.frames) == 0 {
		return 0, false, fmt.Errorf("programlower: no lexical body to finalize")
	}
	frame := b.frames[len(b.frames)-1]
	if frame.terminated {
		if normalValues != 0 {
			return 0, false, fmt.Errorf("programlower: terminal Body has normal Values")
		}
	} else {
		if normalValues == 0 {
			return 0, false, fmt.Errorf("programlower: nonterminal Body has no normal Values")
		}
		normal := b.builder.Normal(frame.span, frame.body, normalValues)
		if normal == 0 {
			return 0, false, fmt.Errorf("programlower: could not create Normal outcome")
		}
		b.roots = append(b.roots, normal)
	}
	if !b.builder.SetBody(frame.body, b.roots[frame.rootMark:]...) {
		return 0, false, fmt.Errorf("programlower: could not finalize Body")
	}
	b.roots = b.roots[:frame.rootMark]
	b.restore(frame.undoMark)
	b.frames = b.frames[:len(b.frames)-1]
	return frame.body, frame.terminated, nil
}

// Append publishes one completed statement relation in the current Body.
func (b *Bodies) Append(term program.Term) error {
	if term == 0 {
		return fmt.Errorf("programlower: could not create Body root")
	}
	b.roots = append(b.roots, term)
	return nil
}

// Child publishes one completed child Body and propagates its terminal state.
func (b *Bodies) Child(body program.Term, terminated bool) error {
	if err := b.Append(body); err != nil {
		return err
	}
	if terminated {
		b.frames[len(b.frames)-1].terminated = true
	}
	return nil
}

// Return records one terminal return in the current Body.
func (b *Bodies) Return(span program.Span, values program.Term) error {
	owner := b.Owner()
	term := b.builder.Return(span, owner, values)
	if term == 0 {
		return fmt.Errorf("programlower: could not lower return")
	}
	b.roots = append(b.roots, term)
	b.frames[len(b.frames)-1].terminated = true
	return nil
}

// Branch records one condition and its two owned Bodies, then propagates the
// exact authored terminal law to the parent Body.
func (b *Bodies) Branch(
	span program.Span,
	condition program.Term,
	whenTrue program.Term,
	whenFalse program.Term,
	thenTerminated bool,
	elseTerminated bool,
	hasAuthoredFalseArm bool,
) error {
	owner := b.Owner()
	branch := b.builder.Branch(span, owner, condition, whenTrue, whenFalse)
	if branch == 0 {
		return fmt.Errorf("programlower: could not create Branch")
	}
	b.roots = append(b.roots, branch)
	if thenTerminated && elseTerminated && hasAuthoredFalseArm {
		b.frames[len(b.frames)-1].terminated = true
	}
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

// CaptureMark identifies the start of one closure capture group.
func (b *Bodies) CaptureMark() int {
	return len(b.captures)
}

// Declare creates and installs one local or formal Cell.
func (b *Bodies) Declare(id symbol.ID, span program.Span) (program.Term, error) {
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
	b.appendCell(cell)
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

// Function completes one closure relation from cells and captures already
// installed by the machine's binder-ordered traversal.
func (b *Bodies) Function(
	span program.Span,
	cellMark int,
	captureMark int,
	varargIndex int,
) (program.Term, error) {
	if len(b.frames) < 2 {
		return 0, fmt.Errorf("programlower: Function has no lexical owner")
	}
	if cellMark < 0 || cellMark > b.cellLen ||
		captureMark < 0 || captureMark > len(b.captures) {
		return 0, fmt.Errorf("programlower: invalid Function construction mark")
	}
	params := b.cellSlice()[cellMark:]
	formals := params
	var vararg program.Term
	if varargIndex >= 0 {
		if varargIndex >= len(params) {
			return 0, fmt.Errorf("programlower: invalid function vararg Cell")
		}
		if varargIndex != len(params)-1 {
			return 0, fmt.Errorf("programlower: function vararg Cell is not final")
		}
		vararg = params[varargIndex]
		formals = params[:varargIndex]
	}
	owner := b.frames[len(b.frames)-2].body
	body := b.frames[len(b.frames)-1].body
	term := b.builder.Function(
		span,
		owner,
		body,
		formals,
		vararg,
		b.captures[captureMark:],
	)
	b.truncateCells(cellMark)
	b.captures = b.captures[:captureMark]
	if term == 0 {
		return 0, fmt.Errorf("programlower: could not create Function")
	}
	return term, nil
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
		len(b.captures) == 0
}
