package lexical

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

type bodyFrame struct {
	body       keyspace.Term
	function   *ast.FunctionExpr
	undoMark   int
	sourceMark int
}

// Entry creates, selects, and enters the canonical chunk Body.
func (b *Bodies) Entry(span source.Span) (keyspace.Term, error) {
	if b == nil || b.collector == nil {
		return 0, fmt.Errorf("lualower: invalid lexical source authority")
	}
	body := b.collector.Body(span)
	if body == 0 {
		return 0, fmt.Errorf("lualower: could not create chunk body")
	}
	if !b.collector.SetEntry(body) {
		return 0, fmt.Errorf("lualower: could not set chunk Entry")
	}
	b.enter(body, nil)
	return body, nil
}

// EnterBlock creates one child Body that inherits its function boundary.
func (b *Bodies) EnterBlock(span source.Span) (keyspace.Term, error) {
	function := b.frames[len(b.frames)-1].function
	return b.enterBody(span, function)
}

// EnterFunction creates one child Body at a new function boundary.
func (b *Bodies) EnterFunction(
	span source.Span,
	function *ast.FunctionExpr,
) (keyspace.Term, error) {
	if function == nil {
		return 0, fmt.Errorf("lualower: nil function boundary")
	}
	return b.enterBody(span, function)
}

func (b *Bodies) enterBody(
	span source.Span,
	function *ast.FunctionExpr,
) (keyspace.Term, error) {
	body := b.collector.Body(span)
	if body == 0 {
		return 0, fmt.Errorf("lualower: could not create Body")
	}
	b.enter(body, function)
	return body, nil
}

func (b *Bodies) enter(body keyspace.Term, function *ast.FunctionExpr) {
	b.frames = append(b.frames, bodyFrame{
		body:       body,
		function:   function,
		undoMark:   len(b.undo),
		sourceMark: len(b.source),
	})
}

// Owner returns the active lexical Body.
func (b *Bodies) Owner() keyspace.Term {
	return b.frames[len(b.frames)-1].body
}

// Finish atomically publishes the current Body, restores its lexical mappings,
// and leaves its parent active.
func (b *Bodies) Finish() (keyspace.Term, error) {
	if len(b.frames) == 0 {
		return 0, fmt.Errorf("lualower: no lexical body to finalize")
	}
	owner := b.Owner()
	for _, current := range b.localSteps {
		if current.owner == owner {
			return 0, fmt.Errorf("lualower: unfinished lexical local transaction")
		}
	}
	frame := b.frames[len(b.frames)-1]
	items := b.source[frame.sourceMark:]
	terms := make([]keyspace.Term, len(items))
	for index, item := range items {
		if item.term == 0 || item.cell != 0 {
			return 0, fmt.Errorf("lualower: unresolved Body source evidence")
		}
		terms[index] = item.term
	}
	if !b.collector.SetBody(frame.body, terms...) {
		return 0, fmt.Errorf("lualower: could not finalize Body")
	}
	b.source = b.source[:frame.sourceMark]
	b.restore(frame.undoMark)
	b.frames = b.frames[:len(b.frames)-1]
	return frame.body, nil
}
