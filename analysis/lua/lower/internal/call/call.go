// Package call owns complete source Call construction.
//
// A Call's evaluation order, receiver insertion, type-value base policy,
// static arguments, open result, and direct-module observation form one
// vertical. Source dispatches closed continuation owners; Call retains every
// typed payload needed to resume its own work.
package call

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	modulelower "github.com/wippyai/go-lua/analysis/lua/lower/internal/module"
	staticlower "github.com/wippyai/go-lua/analysis/lua/lower/internal/static"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/storage"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Writer is the sole Call authority for one unfinished Program. Its private
// steps retain Call-local terms only; no Call state enters Values or the
// parent scheduler.
type Writer struct {
	phase       *continuation.Stack
	exprs       *continuation.Expressions
	values      *eval.Values
	store       *storage.Writer
	static      *staticlower.Writer
	modules     *modulelower.Writer
	calls       *assembly.Collector
	operands    *assembly.Collector
	constructed bool
	binding     *bind.Result
	file        string

	steps []step
}

type stepKind uint8

const (
	stepPlainBase stepKind = iota + 1
	stepPlainAttributeKey
	stepMethodBase
	stepArguments
	stepTypeArguments
)

// step is private Call continuation state. In particular, a receiver and
// callee remain typed Program terms rather than being recovered from a Values
// scratch range after argument evaluation.
type step struct {
	kind stepKind

	call             *ast.FuncCallExpr
	owner            keyspace.Term
	span             source.Span
	attributeSpan    source.Span
	base             keyspace.Term
	callee, receiver keyspace.Term

	callTerm   keyspace.Term
	staticMark int
	index      int
	waiting    bool
}

// New creates the canonical Call lowering authority. Every dependency is a
// concrete semantic owner; there is no callback, adapter, or second path.
func New(
	stack *continuation.Stack,
	expressions *continuation.Expressions,
	values *eval.Values,
	storage *storage.Writer,
	static *staticlower.Writer,
	modules *modulelower.Writer,
	construction *assembly.Collector,
	binding *bind.Result,
	sourceName string,
) *Writer {
	calls := construction
	operands := construction
	return &Writer{
		phase:       stack,
		exprs:       expressions,
		values:      values,
		store:       storage,
		static:      static,
		modules:     modules,
		calls:       calls,
		operands:    operands,
		constructed: construction != nil,
		binding:     binding,
		file:        sourceName,
	}
}

// Schedule starts lowering one syntactically valid Call. It validates the
// exclusive plain/method forms before scheduling either base, so later phases
// never infer a shape from absent fields.
func (w *Writer) Schedule(call *ast.FuncCallExpr, host keyspace.Term, span source.Span) error {
	if !w.ready() || call == nil || host == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid Call authority")
	}
	if call.Method != "" || call.Receiver != nil {
		if call.Method == "" || call.Receiver == nil || call.Func != nil || !call.MethodPosition.Valid() {
			return fmt.Errorf("lualower: invalid method Call shape")
		}
		if err := w.schedule(step{kind: stepMethodBase, call: call, owner: host, span: span}); err != nil {
			return err
		}
		return w.scheduleBase(call.Receiver, host, w.span(call.Receiver))
	}
	if call.Func == nil {
		return fmt.Errorf("lualower: plain Call has no callee")
	}
	if attribute, ok := call.Func.(*ast.AttrGetExpr); ok {
		if attribute == nil || attribute.Object == nil || attribute.Key == nil {
			return fmt.Errorf("lualower: invalid plain Call attribute")
		}
		attributeSpan := w.span(attribute)
		if attributeSpan.File == "" {
			return fmt.Errorf("lualower: unresolved plain Call attribute span")
		}
		if err := w.schedule(step{kind: stepPlainBase, call: call, owner: host, span: span, attributeSpan: attributeSpan}); err != nil {
			return err
		}
		return w.scheduleBase(attribute.Object, host, w.span(attribute.Object))
	}
	if err := w.schedule(step{kind: stepPlainBase, call: call, owner: host, span: span}); err != nil {
		return err
	}
	return w.scheduleBase(call.Func, host, w.span(call.Func))
}

// scheduleBase owns the sole compiler-special Call-base policy. A
// binder-marked type value becomes TypeValue; every other base enters the
// ordinary expression continuation. Source never reinterprets the base.
func (w *Writer) scheduleBase(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if !w.ready() || expr == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid Call base")
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || ident == nil {
		return w.expression(expr, owner, span)
	}
	evidence, marked := w.binding.RuntimeTypeValue(ident)
	if !marked {
		return w.expression(expr, owner, span)
	}
	target, err := w.static.RuntimeTypeTarget(w.span(ident), evidence)
	if err != nil {
		return err
	}
	term := w.operands.TypeValue(w.span(ident), owner, target)
	if term == 0 {
		return fmt.Errorf("lualower: could not create runtime TypeValue")
	}
	w.phase.SetResult(term, false)
	return nil
}

// Run executes exactly one private Call continuation after phase pops Call.
func (w *Writer) Run() error {
	if !w.ready() || len(w.steps) == 0 {
		return fmt.Errorf("lualower: missing Call continuation")
	}
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]
	if current.owner == 0 || current.span.File == "" {
		return fmt.Errorf("lualower: Call continuation lost source identity")
	}

	switch current.kind {
	case stepPlainBase:
		return w.finishPlainBase(current)
	case stepPlainAttributeKey:
		return w.finishPlainAttributeKey(current)
	case stepMethodBase:
		return w.finishMethodBase(current)
	case stepArguments:
		return w.finishArguments(current)
	case stepTypeArguments:
		return w.finishTypeArguments(current)
	default:
		return fmt.Errorf("lualower: invalid Call continuation %d", current.kind)
	}
}

func (w *Writer) finishPlainBase(current step) error {
	base, _ := w.phase.Result()
	if base == 0 || current.call == nil || current.owner == 0 {
		return fmt.Errorf("lualower: missing plain Call callee")
	}
	attribute, isAttribute := current.call.Func.(*ast.AttrGetExpr)
	if !isAttribute {
		return w.scheduleArguments(current.call, current.owner, current.span, base, 0)
	}
	if attribute == nil || attribute.Object == nil || attribute.Key == nil || current.attributeSpan.File == "" {
		return fmt.Errorf("lualower: invalid plain Call attribute")
	}
	switch attribute.KeySyntax {
	case ast.AttrKeyDot:
		name, ok := attribute.Key.(*ast.StringExpr)
		if !ok || name == nil {
			return fmt.Errorf("lualower: dot Call callee key is not a string literal")
		}
		lens, err := w.store.DotLens(current.attributeSpan, current.owner, base, w.span(name), name.Value)
		if err != nil {
			return err
		}
		callee, err := w.store.Read(current.attributeSpan, current.owner, lens)
		if err != nil {
			return err
		}
		return w.scheduleArguments(current.call, current.owner, current.span, callee, 0)
	case ast.AttrKeyIndex:
		current.kind = stepPlainAttributeKey
		current.base = base
		if err := w.schedule(current); err != nil {
			return err
		}
		return w.expression(attribute.Key, current.owner, w.span(attribute.Key))
	default:
		return fmt.Errorf("lualower: unsupported plain Call attribute syntax %d", attribute.KeySyntax)
	}
}

func (w *Writer) finishPlainAttributeKey(current step) error {
	key, _ := w.phase.Result()
	attribute, ok := current.call.Func.(*ast.AttrGetExpr)
	if !ok || attribute == nil || current.base == 0 || key == 0 {
		return fmt.Errorf("lualower: missing plain Call attribute key")
	}
	if current.attributeSpan.File == "" {
		return fmt.Errorf("lualower: plain Call attribute lost source span")
	}
	lens, err := w.store.IndexLens(current.attributeSpan, current.owner, current.base, key, attribute.Key)
	if err != nil {
		return err
	}
	callee, err := w.store.Read(current.attributeSpan, current.owner, lens)
	if err != nil {
		return err
	}
	return w.scheduleArguments(current.call, current.owner, current.span, callee, 0)
}

func (w *Writer) finishMethodBase(current step) error {
	receiver, _ := w.phase.Result()
	if current.call == nil || receiver == 0 || current.owner == 0 || current.call.Method == "" || !current.call.MethodPosition.Valid() {
		return fmt.Errorf("lualower: invalid method Call completion")
	}
	selector := w.methodSelectorSpan(current.span, current.call.MethodPosition)
	lens, err := w.store.DotLens(
		selector,
		current.owner,
		receiver,
		w.positionSpan(current.call.MethodPosition),
		current.call.Method,
	)
	if err != nil {
		return err
	}
	callee, err := w.store.Read(selector, current.owner, lens)
	if err != nil {
		return err
	}
	return w.scheduleArguments(current.call, current.owner, current.span, callee, receiver)
}

func (w *Writer) scheduleArguments(call *ast.FuncCallExpr, owner keyspace.Term, span source.Span, callee, receiver keyspace.Term) error {
	if call == nil || owner == 0 || span.File == "" || callee == 0 || w.values == nil {
		return fmt.Errorf("lualower: invalid Call arguments")
	}
	if err := w.schedule(step{kind: stepArguments, call: call, owner: owner, span: span, callee: callee, receiver: receiver}); err != nil {
		return err
	}
	return w.values.ScheduleValues(call.Args, owner, span)
}

func (w *Writer) finishArguments(current step) error {
	actuals, _ := w.phase.Result()
	if current.call == nil || current.owner == 0 || current.callee == 0 || actuals == 0 {
		return fmt.Errorf("lualower: incomplete Call arguments")
	}
	term := w.calls.DeclareCall(current.span, current.owner, current.callee, current.receiver, actuals)
	if term == 0 {
		return fmt.Errorf("lualower: could not declare Call")
	}
	mark := w.static.Mark()
	if mark < 0 {
		return fmt.Errorf("lualower: invalid Call static argument mark")
	}
	return w.schedule(step{
		kind:       stepTypeArguments,
		call:       current.call,
		owner:      current.owner,
		span:       current.span,
		callTerm:   term,
		staticMark: mark,
	})
}

func (w *Writer) finishTypeArguments(current step) error {
	if current.call == nil || current.callTerm == 0 || current.staticMark < 0 {
		return fmt.Errorf("lualower: invalid Call static argument continuation")
	}
	if current.waiting {
		argument, _ := w.phase.Result()
		if argument == 0 {
			return fmt.Errorf("lualower: missing Call static argument")
		}
		if err := w.static.Append(argument); err != nil {
			return err
		}
		current.waiting = false
	}
	if current.index == len(current.call.TypeArgs) {
		arguments, err := w.static.TakeCallTypeArgs(current.staticMark, len(current.call.TypeArgs))
		if err != nil {
			return err
		}
		if !w.calls.SetCallTypeArgs(current.callTerm, arguments) {
			return fmt.Errorf("lualower: could not finalize Call static arguments")
		}
		if err := w.modules.ObserveCall(current.call, current.span, current.callTerm); err != nil {
			return err
		}
		w.phase.SetResult(current.callTerm, !current.call.AdjustRet)
		return nil
	}
	if current.index < 0 || current.index >= len(current.call.TypeArgs) || current.call.TypeArgs[current.index] == nil {
		return fmt.Errorf("lualower: invalid Call static argument %d", current.index)
	}
	typ := current.call.TypeArgs[current.index]
	current.index++
	current.waiting = true
	if err := w.schedule(current); err != nil {
		return err
	}
	return w.static.ScheduleType(typ, current.callTerm, current.owner, w.span(typ))
}

func (w *Writer) expression(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if expr == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid ordinary Call expression")
	}
	return w.exprs.Push(expr, owner, span)
}

func (w *Writer) schedule(next step) error {
	if next.kind == 0 || next.owner == 0 || next.span.File == "" {
		return fmt.Errorf("lualower: invalid Call continuation identity")
	}
	w.steps = append(w.steps, next)
	w.phase.Push(continuation.Call)
	return nil
}

func (w *Writer) ready() bool {
	return w != nil && w.phase != nil && w.exprs != nil && w.values != nil &&
		w.store != nil && w.static != nil && w.modules != nil && w.constructed && w.binding != nil
}

// Clean reports whether all Call-local continuation state completed.
func (w *Writer) Clean() bool {
	return w != nil && len(w.steps) == 0
}

func (w *Writer) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: w.file}
	}
	span, ok := coord.Build(w.file, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return coord.Invalid(w.file)
	}
	return span
}

func (w *Writer) positionSpan(position ast.Position) source.Span {
	if !position.Valid() {
		if position.Line == 0 && position.Column == 0 && position.EndLine == 0 && position.EndColumn == 0 {
			return source.Span{File: w.file}
		}
		return coord.Invalid(w.file)
	}
	span, ok := coord.Build(w.file, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(w.file)
	}
	return span
}

func (w *Writer) methodSelectorSpan(call source.Span, position ast.Position) source.Span {
	selector := w.positionSpan(position)
	selector.StartLine = call.StartLine
	selector.StartCol = call.StartCol
	return selector
}
