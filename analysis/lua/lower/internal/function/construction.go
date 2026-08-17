package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) begin(fn *ast.FunctionExpr, owner keyspace.Term, span source.Span, done completion) error {
	if w == nil || w.stack == nil || w.collector == nil || w.binding == nil || w.scopes == nil || w.static == nil || fn == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("lualower: missing Function authority")
	}
	phases := w.stack
	if err := w.assertActive(owner); err != nil {
		return err
	}
	function, err := w.scopes.DeclareFunction(span)
	if err != nil {
		return err
	}
	params, err := w.static.BeginFunctionHeader(fn, function)
	if err != nil {
		return err
	}
	w.push(step{kind: stepBegin, fn: fn, function: function, done: done, typeParams: params, slots: w.binding.ParamSlots(fn), captures: w.captures[fn], owner: owner, span: span})
	phases.Push(continuation.Function)
	return nil
}

func (w *Writer) runBegin(current step, phases *continuation.Stack) error {
	if current.fn == nil || current.index < 0 || current.index > len(current.typeParams) {
		return fmt.Errorf("lualower: invalid function generic cursor")
	}
	if current.index == len(current.typeParams) {
		if err := w.assertActive(current.owner); err != nil {
			return err
		}
		body, err := w.scopes.EnterFunction(current.span, current.fn)
		if err != nil {
			return fmt.Errorf("lualower: could not create Function Body: %w", err)
		}
		current.kind, current.body, current.index, current.owner = stepFormals, body, 0, body
		current.mark, current.captureMark = w.scopes.CellMark(), w.scopes.CaptureMark()
		if w.bodies == nil {
			return fmt.Errorf("lualower: missing Function Body preparation inbox")
		}
		w.push(current)
		phases.Push(continuation.Function)
		return w.bodies.PushPrepare(current.fn.Stmts, body, current.span)
	}
	param := current.typeParams[current.index]
	if param.ID == 0 || param.Kind != bind.TypeDeclParam {
		return fmt.Errorf("lualower: invalid function type parameter binding")
	}
	if param.Constraint == nil {
		if err := w.static.FinishParam(param, 0); err != nil {
			return err
		}
		w.push(current.next())
		phases.Push(continuation.Function)
		return nil
	}
	host, ok := w.static.Host(param)
	if !ok {
		return fmt.Errorf("lualower: function type parameter was not predeclared")
	}
	w.push(step{kind: stepFinishGeneric, typeParam: param, index: current.index, fn: current.fn, function: current.function, done: current.done, typeParams: current.typeParams, slots: current.slots, captures: current.captures, owner: current.owner, span: current.span})
	return w.requestStaticType(param.Constraint, host, current.owner, w.span(param.Constraint))
}

func (w *Writer) runRecursiveDeclaredType(current step, phases *continuation.Stack) error {
	if current.local == nil || current.fn == nil || current.slot != 0 || len(current.local.Names) != 1 || len(current.local.Types) > 1 {
		return fmt.Errorf("lualower: invalid recursive local function continuation")
	}
	var declared ast.TypeExpr
	if len(current.local.Types) != 0 {
		declared = current.local.Types[0]
	}
	if declared == nil {
		return w.begin(current.fn, current.owner, current.span, completion{kind: completeRecursiveLocal, local: current.local, cellMark: current.mark, host: current.owner, span: current.completionSpan})
	}
	id, ok := w.binding.LocalSymbolAt(current.local, 0)
	if !ok || id == 0 {
		return fmt.Errorf("lualower: missing recursive local function symbol")
	}
	bound, ok := w.binding.SymbolTypeAnnotation(id)
	if !ok || bound != declared {
		return fmt.Errorf("lualower: mismatched recursive local function type binding")
	}
	host, ok := w.scopes.RetainedCell(current.mark, 0)
	if !ok || host == 0 {
		return fmt.Errorf("lualower: missing recursive local function Cell")
	}
	w.push(step{kind: stepFinishRecursiveType, fn: current.fn, local: current.local, mark: current.mark, host: host, owner: current.owner, span: current.span, completionSpan: current.completionSpan, typeExpr: declared})
	return w.requestStaticType(declared, host, current.owner, w.span(declared))
}
