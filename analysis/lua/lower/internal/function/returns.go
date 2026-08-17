package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
)

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
