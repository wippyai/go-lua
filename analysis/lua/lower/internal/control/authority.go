// Package control owns authored control relations.
package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Writer is the one direct writer for authored terminal and structured control.
type Writer struct {
	flow        *assembly.Collector
	faults      *assembly.Collector
	constructed bool

	// phases carries only closed owner tokens. Expressions and Bodies are the
	// two concrete crossings this owner needs; their payloads never enter the
	// phase stack or a generic route record.
	phases      *continuation.Stack
	binding     *bind.Result
	scopes      *lexical.Bodies
	values      *eval.Values
	expressions *continuation.Expressions
	bodies      *continuation.Bodies
	sourceName  string
	issues      map[ast.Stmt]bind.ControlIssue
	steps       []step

	cellInline   [4]keyspace.Term
	cellOverflow []keyspace.Term
	cellLen      int

	labels     map[*ast.LabelStmt]labelState
	labelCount int
	pending    []pendingFault
}

type stepKind uint8

const (
	finishReturnStep stepKind = iota + 1
	finishIfConditionStep
	finishIfThenStep
	finishIfElseStep
	finishWhileConditionStep
	finishRepeatConditionStep
	finishRepeatControlStep
	finishLoopStep
	numberControlStep
	appendNumberControlStep
	finishGenericControlsStep
)

// step is private control scheduling state. continuation.Stack holds the sole
// global execution token; this payload is never observable as a second IR.
type step struct {
	kind stepKind

	// host is the exact enclosing Body at enqueue time. No continuation
	// discovers its semantic owner from the currently active lexical scope.
	host      keyspace.Term
	parent    keyspace.Term
	span      source.Span
	ret       *ast.ReturnStmt
	ifStmt    *ast.IfStmt
	while     *ast.WhileStmt
	repeat    *ast.RepeatStmt
	number    *ast.NumberForStmt
	generic   *ast.GenericForStmt
	condition keyspace.Term
	whenTrue  keyspace.Term
	whenFalse keyspace.Term
	body      keyspace.Term
	control   keyspace.Term
	cellMark  int
	exprs     []ast.Expr
	terms     []keyspace.Term
	index     int
}

type labelState struct {
	term   keyspace.Term
	placed bool
}

type pendingFault struct {
	span     source.Span
	owner    keyspace.Term
	kind     source.ControlFaultKind
	label    *ast.LabelStmt
	evidence lexical.CellEvidence
}

// New binds the sole control authority to its concrete dependencies.
func New(
	construction *assembly.Collector,
	binding *bind.Result,
	scopes *lexical.Bodies,
	values *eval.Values,
	phases *continuation.Stack,
	expressions *continuation.Expressions,
	bodies *continuation.Bodies,
	sourceName string,
) Writer {
	flow := construction
	faults := construction
	return Writer{
		flow:        flow,
		faults:      faults,
		constructed: construction != nil,
		binding:     binding,
		scopes:      scopes,
		values:      values,
		phases:      phases,
		expressions: expressions,
		bodies:      bodies,
		sourceName:  sourceName,
		issues:      indexIssues(binding),
	}
}

func (w *Writer) ready() error {
	if w == nil || !w.constructed || w.phases == nil || w.binding == nil ||
		w.scopes == nil || w.values == nil || w.expressions == nil || w.bodies == nil {
		return fmt.Errorf("lualower: incomplete control authority")
	}
	return nil
}

// Clean reports whether every pending loop-cell range completed.
func (w *Writer) Clean() bool {
	return w.cellLen == 0 &&
		len(w.cellOverflow) == 0 &&
		len(w.labels) == w.labelCount &&
		len(w.pending) == 0 &&
		len(w.steps) == 0
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

func (w *Writer) chunkSpan(stmts []ast.Stmt) source.Span {
	if len(stmts) == 0 {
		return source.Span{File: w.sourceName}
	}
	first, last := stmts[0], stmts[len(stmts)-1]
	if first == nil || last == nil {
		return coord.Invalid(w.sourceName)
	}
	span, ok := coord.Build(w.sourceName, first.Line(), first.Column(), last.LastLine(), last.LastColumn())
	if !ok {
		return coord.Invalid(w.sourceName)
	}
	return span
}
