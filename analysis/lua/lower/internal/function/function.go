// Package function owns executable Function construction during Program
// lowering. It keeps function-origin validation, closure identity, lexical
// entry, formal/capture construction, and authored static headers together.
// Source dispatches only the closed owner tokens emitted here.
package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	staticlower "github.com/wippyai/go-lua/analysis/lua/lower/internal/static"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/storage"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Writer is the one executable-function authority for an unfinished Program.
// It owns no source-wide scheduler or alternate result channel: continuation.Stack is
// the sole continuation/result crossing with source.
type Writer struct {
	stack       *continuation.Stack
	collector   *assembly.Collector
	binding     *bind.Result
	scopes      *lexical.Bodies
	packs       *eval.Values
	access      *storage.Writer
	static      *staticlower.Writer
	expressions *continuation.Expressions
	bodies      *continuation.Bodies
	statics     *continuation.Statics
	sourceName  string
	captures    map[*ast.FunctionExpr][]bind.Capture
	steps       []step
}

// New creates the executable-function authority. Capture entries are indexed
// once from the binder's boundary stream; no caller-owned capture projection
// is retained or reconstructed per Function.
func New(
	stack *continuation.Stack,
	collector *assembly.Collector,
	binding *bind.Result,
	scopes *lexical.Bodies,
	packs *eval.Values,
	access *storage.Writer,
	static *staticlower.Writer,
	expressions *continuation.Expressions,
	bodies *continuation.Bodies,
	statics *continuation.Statics,
	sourceName string,
) Writer {
	w := Writer{
		stack: stack, collector: collector, binding: binding, scopes: scopes, packs: packs,
		access: access, static: static, expressions: expressions, bodies: bodies,
		statics: statics, sourceName: sourceName,
	}
	if binding != nil {
		binding.ForEachEntryCapture(func(fn *ast.FunctionExpr, capture bind.Capture) bool {
			if w.captures == nil {
				w.captures = make(map[*ast.FunctionExpr][]bind.Capture)
			}
			w.captures[fn] = append(w.captures[fn], capture)
			return true
		})
	}
	return w
}

// ScheduleExpr schedules a function literal or ordinary local initializer.
// Static containment is read directly from static.Writer, then checked against
// the binder's FunctionOrigin; parent composition never carries that judgment.
func (w *Writer) ScheduleExpr(fn *ast.FunctionExpr, host keyspace.Term, span source.Span) error {
	if host == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid Function expression host")
	}
	if err := w.validExprOrigin(fn); err != nil {
		return err
	}
	return w.begin(fn, host, span, completion{kind: completeExpr, host: host, span: span})
}

// ScheduleDef schedules a declaration target before its closure. Plain names
// and dotted targets are Store targets; colon methods evaluate their receiver
// once, then construct the exact selector Lens before closure construction.
func (w *Writer) ScheduleDef(
	stmt *ast.FuncDefStmt,
	host keyspace.Term,
	functionSpan, completionSpan source.Span,
) error {
	if w == nil || w.stack == nil || w.binding == nil || w.scopes == nil || w.access == nil || w.static == nil || stmt == nil || stmt.Name == nil || stmt.Func == nil {
		return fmt.Errorf("lualower: invalid function definition")
	}
	origin, ok := w.binding.FunctionOrigin(stmt.Func)
	if !ok || origin.Func != stmt.Func || origin.Stmt != stmt || origin.Static != (w.static.StaticDepth() > 0) {
		return fmt.Errorf("lualower: unsupported ambiguous function declaration origin")
	}
	if host == 0 || host != w.scopes.Owner() || functionSpan.File == "" || completionSpan.File == "" {
		return fmt.Errorf("lualower: invalid function definition host")
	}
	targetMark := w.access.TargetMark()
	if stmt.Name.Method != "" || stmt.Name.Receiver != nil {
		if w.expressions == nil {
			return fmt.Errorf("lualower: missing expression inbox")
		}
		if err := w.validMethodDef(stmt, origin); err != nil {
			return err
		}
		w.push(step{kind: stepMethodTarget, def: stmt, targetMark: targetMark, owner: host, span: functionSpan, completionSpan: completionSpan, selectorSpan: w.methodSelectorSpan(stmt.Name.Receiver, stmt.Name.MethodPosition), keySpan: w.positionSpan(stmt.Name.MethodPosition)})
		w.stack.Push(continuation.Function)
		return w.expressions.Push(stmt.Name.Receiver, host, w.span(stmt.Name.Receiver))
	}
	if origin.Kind != bind.FunctionOriginDeclaration || !functionTarget(stmt.Name.Func) {
		return fmt.Errorf("lualower: unsupported function definition target")
	}
	w.push(step{kind: stepPlainTarget, def: stmt, targetMark: targetMark, owner: host, span: functionSpan, completionSpan: completionSpan, targetSpan: w.span(stmt.Name.Func)})
	w.stack.Push(continuation.Function)
	return w.access.ScheduleTarget(stmt.Name.Func, host, w.span(stmt.Name.Func))
}

// ScheduleRecursiveLocal lowers the source-classified `local function f`
// declaration. Source owns recognition; Function owns only its predeclaration
// and closure construction semantics.
func (w *Writer) ScheduleRecursiveLocal(
	stmt *ast.LocalAssignStmt,
	host keyspace.Term,
	functionSpan, completionSpan source.Span,
) error {
	if w == nil || w.stack == nil || w.binding == nil || w.scopes == nil || w.static == nil {
		return fmt.Errorf("lualower: missing recursive local function authority")
	}
	if stmt == nil || len(stmt.Exprs) != 1 {
		return fmt.Errorf("lualower: invalid recursive local function")
	}
	fn, ok := stmt.Exprs[0].(*ast.FunctionExpr)
	if !ok || fn == nil {
		return fmt.Errorf("lualower: recursive local function has no Function expression")
	}
	origin, exists := w.binding.FunctionOrigin(fn)
	if !exists || origin.Kind != bind.FunctionOriginLocalAssignment || origin.Func != fn || origin.Stmt != stmt || origin.LocalIndex != 0 || origin.Static != (w.static.StaticDepth() > 0) {
		return fmt.Errorf("lualower: unsupported recursive local function origin")
	}
	id, exists := w.binding.LocalSymbolAt(stmt, 0)
	if !exists || id == 0 || w.scopes.Has(id) {
		return fmt.Errorf("lualower: binder has no recursive local function symbol")
	}
	mark := w.scopes.CellMark()
	if host == 0 || host != w.scopes.Owner() || functionSpan.File == "" || completionSpan.File == "" {
		return fmt.Errorf("lualower: invalid recursive local function host")
	}
	if _, err := w.scopes.Declare(id, w.nameSpan(stmt, 0)); err != nil {
		return fmt.Errorf("lualower: could not predeclare recursive local function: %w", err)
	}
	w.push(step{kind: stepRecursiveDeclaredType, local: stmt, fn: fn, mark: mark, slot: 0, owner: host, span: functionSpan, completionSpan: completionSpan})
	w.stack.Push(continuation.Function)
	return nil
}

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

// Clean reports that no executable-function continuation remains private.
func (w *Writer) Clean() bool { return w != nil && len(w.steps) == 0 }

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
	if slot.ImplicitSelf {
		position, err := w.methodPosition(current.fn)
		if err != nil {
			return err
		}
		span = w.positionSpan(position)
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

func (w *Writer) validExprOrigin(fn *ast.FunctionExpr) error {
	if w == nil || w.binding == nil || w.static == nil || fn == nil {
		return fmt.Errorf("lualower: invalid Function expression")
	}
	origin, ok := w.binding.FunctionOrigin(fn)
	if !ok || origin.Func != fn || origin.Static != (w.static.StaticDepth() > 0) {
		return fmt.Errorf("lualower: unsupported ambiguous Function origin")
	}
	switch origin.Kind {
	case bind.FunctionOriginLiteral:
		return nil
	case bind.FunctionOriginLocalAssignment:
		stmt, ok := origin.Stmt.(*ast.LocalAssignStmt)
		if !ok || stmt == nil || origin.LocalIndex < 0 || origin.LocalIndex >= len(stmt.Exprs) || stmt.Exprs[origin.LocalIndex] != fn {
			return fmt.Errorf("lualower: invalid local Function origin")
		}
		return nil
	default:
		return fmt.Errorf("lualower: unsupported Function expression origin")
	}
}

func (w *Writer) validMethodDef(stmt *ast.FuncDefStmt, origin bind.FunctionOrigin) error {
	if stmt.Name.Method == "" || stmt.Name.Receiver == nil || stmt.Name.Func != nil || !functionTarget(stmt.Name.Receiver) || !stmt.Name.MethodPosition.Valid() ||
		origin.Kind != bind.FunctionOriginMethod || origin.Method != stmt.Name.Method {
		return fmt.Errorf("lualower: invalid method function definition")
	}
	return nil
}

func functionTarget(target ast.Expr) bool {
	for target != nil {
		switch current := target.(type) {
		case *ast.IdentExpr:
			return current != nil && current.Value != ""
		case *ast.AttrGetExpr:
			if current == nil || current.KeySyntax != ast.AttrKeyDot || current.Object == nil || current.Key == nil {
				return false
			}
			key, ok := current.Key.(*ast.StringExpr)
			if !ok || key == nil || key.Value == "" {
				return false
			}
			target = current.Object
		default:
			return false
		}
	}
	return false
}

func (w *Writer) methodPosition(fn *ast.FunctionExpr) (ast.Position, error) {
	origin, ok := w.binding.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginMethod || origin.Func != fn {
		return ast.Position{}, fmt.Errorf("lualower: missing method Function origin")
	}
	stmt, ok := origin.Stmt.(*ast.FuncDefStmt)
	if !ok || stmt == nil || stmt.Name == nil || stmt.Func != fn || stmt.Name.Method == "" || origin.Method != stmt.Name.Method || !stmt.Name.MethodPosition.Valid() {
		return ast.Position{}, fmt.Errorf("lualower: invalid method Function origin")
	}
	return stmt.Name.MethodPosition, nil
}

func (w *Writer) span(holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: w.sourceName}
	}
	span, ok := coord.Build(w.sourceName, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}

func (w *Writer) nameSpan(stmt *ast.LocalAssignStmt, index int) source.Span {
	if stmt != nil && index >= 0 && index < len(stmt.NamePositions) {
		return w.positionSpan(stmt.NamePositions[index])
	}
	return w.span(stmt)
}

func (w *Writer) positionSpan(position ast.Position) source.Span {
	if !position.Valid() {
		if position.Line == 0 && position.Column == 0 && position.EndLine == 0 && position.EndColumn == 0 {
			return source.Span{File: w.sourceName}
		}
		return coord.Invalid(w.sourceName)
	}
	span, ok := coord.Build(w.sourceName, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}

func (w *Writer) methodSelectorSpan(receiver ast.Expr, position ast.Position) source.Span {
	span := w.positionSpan(position)
	if receiver == nil {
		return span
	}
	receiverSpan, ok := coord.Build(span.File, receiver.Line(), receiver.Column(), receiver.LastLine(), receiver.LastColumn())
	if !ok {
		return coord.Invalid(span.File)
	}
	if span.StartLine != receiverSpan.StartLine {
		return span
	}
	startCol := position.Column
	if receiverSpan.EndCol != 0 {
		if receiverSpan.EndCol == ^uint32(0) {
			return coord.Invalid(span.File)
		}
		startCol = int(receiverSpan.EndCol) + 1
	}
	if startCol <= 0 {
		startCol = position.Column
	}
	selector, ok := coord.Build(span.File, int(span.StartLine), startCol, int(span.EndLine), int(span.EndCol))
	if !ok {
		return coord.Invalid(span.File)
	}
	return selector
}
