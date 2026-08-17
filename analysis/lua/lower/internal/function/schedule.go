package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
