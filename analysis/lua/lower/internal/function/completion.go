package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) complete(function keyspace.Term, done completion, phases *continuation.Stack) error {
	if function == 0 || done.host == 0 || done.span.File == "" {
		return fmt.Errorf("lualower: missing completed Function")
	}
	if err := w.assertActive(done.host); err != nil {
		return err
	}
	switch done.kind {
	case completeExpr:
		phases.SetResult(function, false)
		return nil
	case completeDefinition:
		if done.def == nil || done.host == 0 || done.span.File == "" {
			return fmt.Errorf("lualower: invalid function definition completion")
		}
		values, err := w.singletonValues(done.span, done.host, function)
		if err != nil {
			return err
		}
		assign, err := w.access.Assign(done.span, done.host, done.targetMark, values, nil)
		if err != nil {
			return err
		}
		if err := w.scopes.Append(assign); err != nil {
			return err
		}
		phases.SetResult(assign, false)
		return nil
	case completeRecursiveLocal:
		if done.local == nil || done.cellMark < 0 || done.host == 0 || done.span.File == "" {
			return fmt.Errorf("lualower: invalid recursive local function completion")
		}
		values, err := w.singletonValues(done.span, done.host, function)
		if err != nil {
			return err
		}
		if err := w.scopes.Bind(done.cellMark, done.span, values); err != nil {
			return err
		}
		phases.SetResult(function, false)
		return nil
	default:
		return fmt.Errorf("lualower: invalid Function completion")
	}
}

func (w *Writer) requestStaticType(typ ast.TypeExpr, host, body keyspace.Term, span source.Span) error {
	if w == nil || w.stack == nil || w.statics == nil || typ == nil || host == 0 || body == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid Function static type request")
	}
	w.stack.Push(continuation.Function)
	return w.statics.PushType(typ, host, body, span)
}

func (w *Writer) assertActive(body keyspace.Term) error {
	if w == nil || w.scopes == nil || body == 0 || w.scopes.Owner() != body {
		return fmt.Errorf("lualower: Function continuation crossed Body boundary")
	}
	return nil
}

func (w *Writer) singletonValues(span source.Span, owner, value keyspace.Term) (keyspace.Term, error) {
	if w == nil || w.packs == nil || owner == 0 || value == 0 {
		return 0, fmt.Errorf("lualower: invalid Function result Values")
	}
	return w.packs.Singleton(span, owner, value)
}

func (w *Writer) push(next step) { w.steps = append(w.steps, next) }

func (w *Writer) pop() step {
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]
	return current
}
