package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
)

func (w *Writer) runFormal(current step, phases *continuation.Stack) error {
	if current.fn == nil || current.index < 0 || current.index > len(current.slots) {
		return fmt.Errorf("lualower: invalid function formal cursor")
	}
	if current.index == len(current.slots) {
		w.push(step{kind: stepCaptures, fn: current.fn, function: current.function, done: current.done, slots: current.slots, captures: current.captures, mark: current.mark, captureMark: current.captureMark, body: current.body, owner: current.owner, span: current.span})
		phases.Push(continuation.Function)
		return nil
	}
	slot := current.slots[current.index]
	if slot.Symbol == 0 || w.scopes.Has(slot.Symbol) {
		return fmt.Errorf("lualower: invalid binder symbol for function formal %q", slot.Name)
	}
	span := w.positionSpan(slot.Position)
	if span.File == "" || slot.ImplicitSelf && !slot.Position.Valid() {
		return fmt.Errorf("lualower: missing binder position for function formal %q", slot.Name)
	}
	host, err := w.scopes.Declare(slot.Symbol, span)
	if err != nil {
		return fmt.Errorf("lualower: could not create function formal Cell: %w", err)
	}
	if slot.ImplicitSelf {
		if decl, ok := w.binding.MethodReceiverType(current.fn); ok {
			if err := w.static.DeclareImplicitSelfType(host, span, decl); err != nil {
				return err
			}
		}
	}
	w.push(current.next())
	phases.Push(continuation.Function)
	return nil
}

func (w *Writer) runCapture(current step, phases *continuation.Stack) error {
	if current.index < 0 || current.index > len(current.captures) {
		return fmt.Errorf("lualower: invalid function capture cursor")
	}
	if current.index != len(current.captures) {
		capture := current.captures[current.index]
		outer, ok := w.scopes.Resolve(capture.Captured)
		if !ok || outer == 0 {
			return fmt.Errorf("lualower: missing outer Cell for capture %q", capture.CapturedName)
		}
		if _, err := w.scopes.Capture(capture.Captured, current.span, outer); err != nil {
			return fmt.Errorf("lualower: could not create function capture Cell: %w", err)
		}
		w.push(current.next())
		phases.Push(continuation.Function)
		return nil
	}
	vararg := -1
	for index, slot := range current.slots {
		if !slot.Vararg {
			continue
		}
		if vararg >= 0 || index != len(current.slots)-1 {
			return fmt.Errorf("lualower: invalid function vararg Cell")
		}
		vararg = index
	}
	if err := w.scopes.FillFunction(current.function, current.mark, current.captureMark, vararg); err != nil {
		return err
	}
	w.push(step{kind: stepHeaderFormal, fn: current.fn, function: current.function, done: current.done, slots: current.slots, index: 0, staticMark: w.static.Mark(), body: current.body, owner: current.owner, span: current.span})
	phases.Push(continuation.Function)
	return nil
}

func (w *Writer) runHeaderFormal(current step, phases *continuation.Stack) error {
	if current.fn == nil || current.index < 0 || current.index > len(current.slots) {
		return fmt.Errorf("lualower: invalid function header parameter cursor")
	}
	if current.index == len(current.slots) {
		current.kind, current.index = stepHeaderReturns, 0
		w.push(current)
		phases.Push(continuation.Function)
		return nil
	}
	slot := current.slots[current.index]
	current.index++
	if slot.Type == nil {
		w.push(current)
		phases.Push(continuation.Function)
		return nil
	}
	if slot.Symbol == 0 {
		return fmt.Errorf("lualower: missing typed function parameter symbol")
	}
	bound, ok := w.binding.SymbolTypeAnnotation(slot.Symbol)
	if !ok || bound != slot.Type {
		return fmt.Errorf("lualower: mismatched function parameter type binding")
	}
	host, ok := w.scopes.Resolve(slot.Symbol)
	if !ok || host == 0 {
		return fmt.Errorf("lualower: missing typed function parameter Cell")
	}
	current.kind, current.host, current.typeExpr = stepFinishFormalType, host, slot.Type
	w.push(current)
	return w.requestStaticType(slot.Type, host, current.body, w.span(slot.Type))
}

func (w *Writer) runHeaderReturns(current step, phases *continuation.Stack) error {
	if current.fn == nil || current.index < 0 || current.index > len(current.fn.ReturnTypes) {
		return fmt.Errorf("lualower: invalid function return cursor")
	}
	if current.index == len(current.fn.ReturnTypes) {
		if err := w.static.FinishFunctionReturns(current.fn, current.function, current.staticMark, len(current.fn.ReturnTypes)); err != nil {
			return err
		}
		if w.bodies == nil {
			return fmt.Errorf("lualower: missing Function statements inbox")
		}
		w.push(step{kind: stepRequestClose, fn: current.fn, function: current.function, done: current.done, body: current.body, owner: current.owner, span: current.span})
		phases.Push(continuation.Function)
		return w.bodies.PushStatements(current.fn.Stmts, 0, current.body, current.span)
	}
	typ := current.fn.ReturnTypes[current.index]
	if typ == nil {
		return fmt.Errorf("lualower: absent function return at index %d", current.index)
	}
	current.index++
	current.kind, current.typeExpr = stepFinishReturnType, typ
	w.push(current)
	return w.requestStaticType(typ, current.function, current.body, w.span(typ))
}
