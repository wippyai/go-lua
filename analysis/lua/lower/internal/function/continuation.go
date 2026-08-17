package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/storage"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

type stepKind uint8

const (
	stepPlainTarget stepKind = iota + 1
	stepMethodTarget
	stepRecursiveDeclaredType
	stepBegin
	stepFinishGeneric
	stepFormals
	stepCaptures
	stepHeaderFormal
	stepFinishFormalType
	stepHeaderReturns
	stepFinishReturnType
	stepFinishRecursiveType
	stepRequestClose
	stepCloseBody
)

type completionKind uint8

const (
	completeExpr completionKind = iota + 1
	completeDefinition
	completeRecursiveLocal
)

type completion struct {
	kind       completionKind
	def        *ast.FuncDefStmt
	local      *ast.LocalAssignStmt
	targetMark storage.TargetMark
	cellMark   int
	host       keyspace.Term
	span       source.Span
}

type step struct {
	kind           stepKind
	fn             *ast.FunctionExpr
	def            *ast.FuncDefStmt
	local          *ast.LocalAssignStmt
	done           completion
	typeParams     []bind.TypeDecl
	typeParam      bind.TypeDecl
	slots          []bind.ParamSlot
	captures       []bind.Capture
	index          int
	mark           int
	targetMark     storage.TargetMark
	captureMark    int
	staticMark     int
	host           keyspace.Term
	owner          keyspace.Term
	body           keyspace.Term
	function       keyspace.Term
	typeExpr       ast.TypeExpr
	span           source.Span
	completionSpan source.Span
	targetSpan     source.Span
	selectorSpan   source.Span
	keySpan        source.Span
	slot           int
}

func (s step) next() step { s.index++; return s }

// Run advances one private function continuation. Each child crossing leaves
// a closed Function token beneath its typed inbox token; no callback,
// interface, or generic route is involved.
func (w *Writer) Run() error {
	if w == nil || w.stack == nil || len(w.steps) == 0 {
		return fmt.Errorf("lualower: invalid function continuation")
	}
	phases := w.stack
	current := w.pop()
	if current.owner == 0 {
		return fmt.Errorf("lualower: Function continuation has no active Body")
	}
	if err := w.assertActive(current.owner); err != nil {
		return err
	}
	switch current.kind {
	case stepPlainTarget:
		target, open := phases.Result()
		if target == 0 || open {
			return fmt.Errorf("lualower: invalid function definition target result")
		}
		if err := w.access.RememberTarget(current.targetSpan, target); err != nil {
			return err
		}
		return w.begin(current.def.Func, current.owner, current.span, completion{kind: completeDefinition, def: current.def, targetMark: current.targetMark, host: current.owner, span: current.completionSpan})
	case stepMethodTarget:
		receiver, open := phases.Result()
		if receiver == 0 || open {
			return fmt.Errorf("lualower: invalid method function receiver result")
		}
		target, err := w.access.DotLens(current.selectorSpan, current.owner, receiver, current.keySpan, current.def.Name.Method)
		if err != nil {
			return err
		}
		if err := w.access.RememberTarget(current.selectorSpan, target); err != nil {
			return err
		}
		return w.begin(current.def.Func, current.owner, current.span, completion{kind: completeDefinition, def: current.def, targetMark: current.targetMark, host: current.owner, span: current.completionSpan})
	case stepRecursiveDeclaredType:
		return w.runRecursiveDeclaredType(current, phases)
	case stepBegin:
		return w.runBegin(current, phases)
	case stepFinishGeneric:
		term, open := phases.Result()
		if term == 0 || open {
			return fmt.Errorf("lualower: invalid function generic constraint result")
		}
		if err := w.static.FinishParam(current.typeParam, term); err != nil {
			return err
		}
		w.push(step{kind: stepBegin, fn: current.fn, function: current.function, done: current.done, typeParams: current.typeParams, slots: current.slots, captures: current.captures, index: current.index + 1, owner: current.owner, span: current.span})
		phases.Push(continuation.Function)
		return nil
	case stepFormals:
		return w.runFormal(current, phases)
	case stepCaptures:
		return w.runCapture(current, phases)
	case stepRequestClose:
		if w.bodies == nil || current.body == 0 {
			return fmt.Errorf("lualower: missing Function Body closure inbox")
		}
		w.push(step{kind: stepCloseBody, function: current.function, done: current.done, body: current.body, owner: current.done.host, span: current.span})
		phases.Push(continuation.Function)
		return w.bodies.PushClose(current.body, current.span)
	case stepHeaderFormal:
		return w.runHeaderFormal(current, phases)
	case stepFinishFormalType:
		term, open := phases.Result()
		if term == 0 || open {
			return fmt.Errorf("lualower: invalid function parameter type result")
		}
		if err := w.static.DeclareCellType(current.host, current.typeExpr, term); err != nil {
			return err
		}
		current.kind, current.host, current.typeExpr = stepHeaderFormal, 0, nil
		w.push(current)
		phases.Push(continuation.Function)
		return nil
	case stepHeaderReturns:
		return w.runHeaderReturns(current, phases)
	case stepFinishReturnType:
		term, open := phases.Result()
		if term == 0 || open {
			return fmt.Errorf("lualower: invalid function return type result")
		}
		if err := w.static.Append(term); err != nil {
			return err
		}
		current.kind, current.typeExpr = stepHeaderReturns, nil
		w.push(current)
		phases.Push(continuation.Function)
		return nil
	case stepFinishRecursiveType:
		term, open := phases.Result()
		if term == 0 || open {
			return fmt.Errorf("lualower: invalid recursive local function type result")
		}
		if err := w.static.DeclareCellType(current.host, current.typeExpr, term); err != nil {
			return err
		}
		return w.begin(current.fn, current.owner, current.span, completion{kind: completeRecursiveLocal, local: current.local, cellMark: current.mark, host: current.owner, span: current.completionSpan})
	case stepCloseBody:
		body, open := phases.Result()
		if body == 0 || open || body != current.body {
			return fmt.Errorf("lualower: invalid function Body completion")
		}
		return w.complete(current.function, current.done, phases)
	default:
		return fmt.Errorf("lualower: invalid function continuation %d", current.kind)
	}
}
