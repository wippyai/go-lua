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
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
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

// CellMark identifies the start of one loop's pending per-iteration Cells.
func (w *Writer) CellMark() int {
	return w.cellLen
}

// RememberCell retains one loop Cell in declaration order.
func (w *Writer) RememberCell(cell keyspace.Term) error {
	if cell == 0 {
		return fmt.Errorf("lualower: could not retain loop Cell")
	}
	w.appendCell(cell)
	return nil
}

// PredeclareLabel allocates one addressable source label before the Body is
// traversed, so both forward and outward Gotos can carry their final typed
// target without an unresolved Program relation.
func (w *Writer) PredeclareLabel(
	stmt *ast.LabelStmt,
	span source.Span,
	owner keyspace.Term,
) error {
	if stmt == nil {
		return fmt.Errorf("lualower: cannot predeclare nil Label")
	}
	if w.labels == nil {
		w.labels = make(map[*ast.LabelStmt]labelState)
	}
	if _, exists := w.labels[stmt]; exists {
		return fmt.Errorf("lualower: duplicate Label predeclaration")
	}
	label := w.flow.Label(span, owner)
	if label == 0 {
		return fmt.Errorf("lualower: could not create Label")
	}
	w.labels[stmt] = labelState{term: label}
	return nil
}

// Label returns one predeclared Label at its authored source turn. The caller
// appends the existing identity to the active Body's sole source sequence.
func (w *Writer) Label(stmt *ast.LabelStmt) (keyspace.Term, error) {
	state, ok := w.labels[stmt]
	if !ok || state.term == 0 {
		return 0, fmt.Errorf("lualower: Label was not predeclared")
	}
	if state.placed {
		return 0, fmt.Errorf("lualower: Label occurred twice")
	}
	state.placed = true
	w.labels[stmt] = state
	w.labelCount++
	return state.term, nil
}

// Goto records one resolved non-local transfer. Label names and resolution
// remain transient binder concerns; Program receives only the exact Label.
func (w *Writer) Goto(
	span source.Span,
	owner keyspace.Term,
	target *ast.LabelStmt,
) (keyspace.Term, error) {
	state, ok := w.labels[target]
	if !ok || state.term == 0 {
		return 0, fmt.Errorf("lualower: Goto target was not predeclared")
	}
	term := w.flow.Goto(span, owner, state.term)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower goto")
	}
	return term, nil
}

// Fault records one binder-rejected control statement. label is the prior
// valid declaration for a duplicate, or the resolved destination for an
// enters-local goto; blocker is that exact entered Cell.
func (w *Writer) Fault(
	span source.Span,
	owner keyspace.Term,
	kind source.ControlFaultKind,
	label *ast.LabelStmt,
	blocker keyspace.Term,
) (keyspace.Term, error) {
	var labelTerm keyspace.Term
	if label != nil {
		state, ok := w.labels[label]
		if !ok || state.term == 0 {
			return 0, fmt.Errorf("lualower: ControlFault Label was not predeclared")
		}
		labelTerm = state.term
	}
	term := w.faults.ControlFault(span, owner, kind, labelTerm, blocker)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not create ControlFault")
	}
	return term, nil
}

// DeferFault retains one control-owned judgment whose blocker Cell is declared
// later in the same Body. Lexical evidence reserves source order but carries no
// callback and cannot construct the ControlFault itself.
func (w *Writer) DeferFault(
	span source.Span,
	owner keyspace.Term,
	kind source.ControlFaultKind,
	label *ast.LabelStmt,
	evidence lexical.CellEvidence,
) error {
	if owner == 0 || kind != source.ControlFaultGotoEntersLocal || label == nil {
		return fmt.Errorf("lualower: invalid pending ControlFault")
	}
	w.pending = append(w.pending, pendingFault{
		span: span, owner: owner, kind: kind, label: label, evidence: evidence,
	})
	return nil
}

// ResolveFaults constructs every pending fault owned by the active lexical
// Body and fills its reserved source turn. Source runs it immediately before
// lexical Finish.
func (w *Writer) ResolveFaults(body keyspace.Term, scopes *lexical.Bodies) error {
	if scopes == nil {
		return fmt.Errorf("lualower: nil lexical authority for ControlFault")
	}
	if body == 0 || scopes.Owner() != body {
		return fmt.Errorf("lualower: ControlFault Body is not active")
	}
	write := 0
	for index := range w.pending {
		fault := w.pending[index]
		if fault.owner != body {
			w.pending[write] = fault
			write++
			continue
		}
		blocker, err := scopes.ResolveCell(fault.evidence)
		if err != nil {
			return err
		}
		term, err := w.Fault(fault.span, fault.owner, fault.kind, fault.label, blocker)
		if err != nil {
			return err
		}
		if err := scopes.Fill(fault.evidence, term); err != nil {
			return err
		}
	}
	w.pending = w.pending[:write]
	return nil
}

// Return records a function exit.
func (w *Writer) Return(
	span source.Span,
	owner keyspace.Term,
	values keyspace.Term,
) (keyspace.Term, error) {
	term := w.flow.Return(span, owner, values)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower return")
	}
	return term, nil
}

// Break records a loop exit. Seal resolves its nearest same-function Loop.
func (w *Writer) Break(
	span source.Span,
	owner keyspace.Term,
) (keyspace.Term, error) {
	term := w.flow.Break(span, owner)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower break")
	}
	return term, nil
}

// Branch records one authored selection.
func (w *Writer) Branch(
	span source.Span,
	owner keyspace.Term,
	condition keyspace.Term,
	whenTrue keyspace.Term,
	whenFalse keyspace.Term,
) (keyspace.Term, error) {
	term := w.flow.Branch(span, owner, condition, whenTrue, whenFalse)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not create Branch")
	}
	return term, nil
}

// Loop publishes one structurally owned loop Body and consumes its pending
// per-iteration Cells. Seal owns all exit and recurrence judgments.
func (w *Writer) Loop(
	span source.Span,
	owner keyspace.Term,
	body keyspace.Term,
	control keyspace.Term,
	cellMark int,
	loopKind flowkind.LoopKind,
) (keyspace.Term, error) {
	if cellMark < 0 || cellMark > w.cellLen {
		return 0, fmt.Errorf("lualower: invalid loop Cell mark")
	}
	term := w.flow.Loop(
		span,
		owner,
		body,
		control,
		w.cellSlice()[cellMark:],
		loopKind,
	)
	w.truncateCells(cellMark)
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower loop")
	}
	return term, nil
}

// Clean reports whether every pending loop-cell range completed.
func (w *Writer) Clean() bool {
	return w.cellLen == 0 &&
		len(w.cellOverflow) == 0 &&
		len(w.labels) == w.labelCount &&
		len(w.pending) == 0 &&
		len(w.steps) == 0
}

func (w *Writer) appendCell(cell keyspace.Term) {
	if w.cellLen < len(w.cellInline) {
		w.cellInline[w.cellLen] = cell
		w.cellLen++
		return
	}
	if w.cellLen == len(w.cellInline) {
		w.cellOverflow = append(w.cellOverflow[:0], w.cellInline[:]...)
	}
	w.cellOverflow = append(w.cellOverflow, cell)
	w.cellLen++
}

func (w *Writer) cellSlice() []keyspace.Term {
	if w.cellLen <= len(w.cellInline) {
		return w.cellInline[:w.cellLen]
	}
	return w.cellOverflow[:w.cellLen]
}

func (w *Writer) truncateCells(mark int) {
	w.cellLen = mark
	if mark <= len(w.cellInline) {
		w.cellOverflow = w.cellOverflow[:0]
		return
	}
	w.cellOverflow = w.cellOverflow[:mark]
}

// Predeclare publishes the addressable Labels in one already-entered lexical
// Body. The binder owns which declarations are invalid; a duplicate never
// receives a second Program Label.
func (w *Writer) Predeclare(stmts []ast.Stmt, body keyspace.Term) error {
	if err := w.ready(); err != nil {
		return err
	}
	if body == 0 || body != w.scopes.Owner() {
		return fmt.Errorf("lualower: Label predeclaration crossed Body boundary")
	}
	for _, stmt := range stmts {
		label, ok := stmt.(*ast.LabelStmt)
		if !ok {
			continue
		}
		if label == nil {
			return fmt.Errorf("lualower: absent Label")
		}
		if issue, invalid := w.issues[label]; invalid && issue.Kind == bind.ControlIssueDuplicateLabel {
			continue
		}
		if err := w.PredeclareLabel(label, w.span(label), body); err != nil {
			return err
		}
	}
	return nil
}

// ScheduleReturn schedules exactly one return statement classified by source.
func (w *Writer) ScheduleReturn(stmt *ast.ReturnStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: absent Return")
	}
	w.push(step{kind: finishReturnStep, host: host, span: span, ret: stmt})
	return w.valueList(stmt.Exprs, host, span)
}

// ScheduleBreak writes a legal break immediately, or its exact binder fault.
func (w *Writer) ScheduleBreak(stmt *ast.BreakStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: absent Break")
	}
	if issue, invalid := w.issues[stmt]; invalid {
		return w.appendFault(span, host, issue)
	}
	term, err := w.Break(span, host)
	if err != nil {
		return err
	}
	return w.appendTo(host, term)
}

// ScheduleLabel writes one predeclared label at its authored source turn.
func (w *Writer) ScheduleLabel(stmt *ast.LabelStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: absent Label")
	}
	if issue, invalid := w.issues[stmt]; invalid {
		return w.appendFault(span, host, issue)
	}
	term, err := w.Label(stmt)
	if err != nil {
		return err
	}
	return w.appendTo(host, term)
}

// ScheduleGoto writes one resolved non-local transfer or its exact fault.
func (w *Writer) ScheduleGoto(stmt *ast.GotoStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: absent Goto")
	}
	if issue, invalid := w.issues[stmt]; invalid {
		return w.appendFault(span, host, issue)
	}
	target, ok := w.binding.GotoTarget(stmt)
	if !ok {
		return fmt.Errorf("lualower: binder has no legal target for goto")
	}
	term, err := w.Goto(span, host, target)
	if err != nil {
		return err
	}
	return w.appendTo(host, term)
}

// ScheduleIf schedules an authored conditional from its exact enclosing Body.
func (w *Writer) ScheduleIf(stmt *ast.IfStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || stmt.Condition == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: invalid If")
	}
	w.push(step{kind: finishIfConditionStep, host: host, span: span, ifStmt: stmt})
	return w.expression(stmt.Condition, host, w.span(stmt.Condition))
}

// ScheduleWhile schedules an authored loop condition from its exact host.
func (w *Writer) ScheduleWhile(stmt *ast.WhileStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || stmt.Condition == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: invalid While")
	}
	w.push(step{kind: finishWhileConditionStep, host: host, span: span, while: stmt})
	return w.expression(stmt.Condition, host, w.span(stmt.Condition))
}

// ScheduleRepeat begins the body before scheduling its repeat-until condition.
func (w *Writer) ScheduleRepeat(stmt *ast.RepeatStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: absent Repeat")
	}
	return w.beginRepeat(stmt, host, span)
}

// ScheduleNumberFor schedules the exact authored numeric loop controls.
func (w *Writer) ScheduleNumberFor(stmt *ast.NumberForStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: absent NumberFor")
	}
	return w.beginNumberFor(stmt, host, span)
}

// ScheduleGenericFor schedules the exact authored generic loop controls.
func (w *Writer) ScheduleGenericFor(stmt *ast.GenericForStmt, host keyspace.Term, span source.Span) error {
	if err := w.ready(); err != nil {
		return err
	}
	if stmt == nil || !w.activeHost(host, span) {
		return fmt.Errorf("lualower: absent GenericFor")
	}
	return w.beginGenericFor(stmt, host, span)
}

func (w *Writer) activeHost(host keyspace.Term, span source.Span) bool {
	return host != 0 && span.File != "" && host == w.scopes.Owner()
}

// Run advances exactly one private control continuation. Cross-vertical work
// is requested through its typed inbox; its result returns through continuation.Result.
func (w *Writer) Run() error {
	if err := w.ready(); err != nil {
		return err
	}
	if len(w.steps) == 0 {
		return fmt.Errorf("lualower: missing control continuation")
	}
	last := len(w.steps) - 1
	current := w.steps[last]
	w.steps = w.steps[:last]

	switch current.kind {
	case finishReturnStep:
		return w.finishReturn(current)
	case finishIfConditionStep:
		return w.finishIfCondition(current)
	case finishIfThenStep:
		return w.finishIfThen(current)
	case finishIfElseStep:
		return w.finishIfElse(current)
	case finishWhileConditionStep:
		return w.finishWhileCondition(current)
	case finishRepeatConditionStep:
		return w.finishRepeatCondition(current)
	case finishRepeatControlStep:
		return w.finishRepeatControl(current)
	case finishLoopStep:
		return w.finishLoop(current)
	case numberControlStep:
		return w.runNumberControls(current)
	case appendNumberControlStep:
		return w.appendNumberControl(current)
	case finishGenericControlsStep:
		return w.finishGenericControls(current)
	default:
		return fmt.Errorf("lualower: unknown control continuation %d", current.kind)
	}
}

func (w *Writer) appendFault(span source.Span, owner keyspace.Term, issue bind.ControlIssue) error {
	var kind source.ControlFaultKind
	var label *ast.LabelStmt
	switch issue.Kind {
	case bind.ControlIssueDuplicateLabel:
		kind, label = source.ControlFaultDuplicateLabel, issue.Previous
	case bind.ControlIssueUndefinedLabel:
		kind = source.ControlFaultUndefinedGoto
	case bind.ControlIssueBreakOutsideLoop:
		kind = source.ControlFaultBreakOutsideLoop
	case bind.ControlIssueGotoEntersLocal:
		kind, label = source.ControlFaultGotoEntersLocal, issue.Label
		evidence, err := w.scopes.ReserveCell(issue.Local)
		if err != nil {
			return err
		}
		return w.DeferFault(span, owner, kind, label, evidence)
	default:
		return fmt.Errorf("lualower: unknown control issue %d", issue.Kind)
	}
	term, err := w.Fault(span, owner, kind, label, 0)
	if err != nil {
		return err
	}
	return w.appendTo(owner, term)
}

func (w *Writer) finishReturn(current step) error {
	values, _ := w.phases.Result()
	if current.ret == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: return continuation lost its host Body")
	}
	term, err := w.Return(current.span, current.host, values)
	if err != nil {
		return err
	}
	return w.appendTo(current.host, term)
}

func (w *Writer) finishIfCondition(current step) error {
	stmt := current.ifStmt
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid if continuation")
	}
	condition, _ := w.phases.Result()
	return w.openBody(stmt.Then, w.chunkSpan(stmt.Then), step{
		kind: finishIfThenStep, host: current.host, span: current.span,
		ifStmt: stmt, condition: condition,
	})
}

func (w *Writer) finishIfThen(current step) error {
	whenTrue, _ := w.phases.Result()
	if whenTrue == 0 || whenTrue != current.body {
		return fmt.Errorf("lualower: mismatched then Body")
	}
	stmt := current.ifStmt
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid then continuation")
	}
	return w.openBody(stmt.Else, w.chunkSpan(stmt.Else), step{
		kind: finishIfElseStep, host: current.host, span: current.span,
		ifStmt:    stmt,
		condition: current.condition, whenTrue: whenTrue,
	})
}

func (w *Writer) finishIfElse(current step) error {
	whenFalse, _ := w.phases.Result()
	if whenFalse == 0 || whenFalse != current.body {
		return fmt.Errorf("lualower: mismatched else Body")
	}
	if current.ifStmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: branch continuation lost its host Body")
	}
	term, err := w.Branch(current.span, current.host, current.condition, current.whenTrue, whenFalse)
	if err != nil {
		return err
	}
	return w.appendTo(current.host, term)
}

func (w *Writer) finishWhileCondition(current step) error {
	stmt := current.while
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid while continuation")
	}
	control, _ := w.phases.Result()
	return w.openLoopBody(stmt.Stmts, w.chunkSpan(stmt.Stmts), step{
		kind: finishLoopStep, host: current.host, span: current.span, while: stmt,
		control: control, cellMark: w.CellMark(),
	})
}

func (w *Writer) beginRepeat(stmt *ast.RepeatStmt, host keyspace.Term, span source.Span) error {
	if stmt == nil || stmt.Condition == nil {
		return fmt.Errorf("lualower: invalid repeat statement")
	}
	if host == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: repeat began outside its host Body")
	}
	// The condition is evaluated after its body statements but before that body
	// closes, preserving repeat-until visibility of body-local Cells.
	body, err := w.scopes.EnterBlock(w.chunkSpan(stmt.Stmts))
	if err != nil {
		return fmt.Errorf("lualower: could not create repeat Body: %w", err)
	}
	w.push(step{
		kind: finishRepeatConditionStep, host: body, parent: host, span: span,
		repeat: stmt, body: body, cellMark: w.CellMark(),
	})
	bodySpan := w.chunkSpan(stmt.Stmts)
	if err := w.bodies.PushStatements(stmt.Stmts, 0, body, bodySpan); err != nil {
		return err
	}
	return w.bodies.PushPrepare(stmt.Stmts, body, bodySpan)
}

func (w *Writer) finishRepeatCondition(current step) error {
	stmt := current.repeat
	if stmt == nil || stmt.Condition == nil || current.body != current.host || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid repeat continuation")
	}
	w.push(step{
		kind: finishRepeatControlStep, host: current.host, parent: current.parent,
		span: current.span, repeat: stmt, body: current.body, cellMark: current.cellMark,
	})
	return w.expression(stmt.Condition, current.host, w.span(stmt.Condition))
}

func (w *Writer) finishRepeatControl(current step) error {
	stmt := current.repeat
	if stmt == nil || current.body != current.host || current.parent == 0 || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid repeat close continuation")
	}
	control, _ := w.phases.Result()
	w.push(step{
		kind: finishLoopStep, host: current.parent, span: current.span,
		repeat: stmt, body: current.body, control: control, cellMark: current.cellMark,
	})
	return w.bodies.PushClose(current.body, w.chunkSpan(stmt.Stmts))
}

func (w *Writer) beginNumberFor(stmt *ast.NumberForStmt, host keyspace.Term, span source.Span) error {
	if stmt == nil || stmt.Init == nil || stmt.Limit == nil {
		return fmt.Errorf("lualower: invalid numeric for statement")
	}
	exprs := []ast.Expr{stmt.Init, stmt.Limit}
	if stmt.Step != nil {
		exprs = append(exprs, stmt.Step)
	}
	if host == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: numeric for began outside its host Body")
	}
	w.push(step{kind: numberControlStep, host: host, span: span, number: stmt, exprs: exprs})
	return nil
}

func (w *Writer) runNumberControls(current step) error {
	stmt := current.number
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid numeric for continuation")
	}
	if current.index == len(current.exprs) {
		control, err := w.values.Fixed(current.span, current.host, current.terms)
		if err != nil {
			return err
		}
		return w.openLoopBody(stmt.Stmts, w.chunkSpan(stmt.Stmts), step{
			kind: finishLoopStep, host: current.host, span: current.span, number: stmt,
			control: control, cellMark: w.CellMark(),
		})
	}
	if current.index < 0 || current.index > len(current.exprs) {
		return fmt.Errorf("lualower: invalid numeric for cursor")
	}
	next := current
	next.index++
	w.push(step{kind: appendNumberControlStep, host: current.host, span: current.span, number: stmt, exprs: next.exprs, terms: next.terms, index: next.index})
	expr := current.exprs[current.index]
	return w.expression(expr, current.host, w.span(expr))
}

func (w *Writer) appendNumberControl(current step) error {
	term, _ := w.phases.Result()
	if term == 0 {
		return fmt.Errorf("lualower: absent numeric for control")
	}
	current.terms = append(current.terms, term)
	if current.number == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: numeric control continuation lost its host Body")
	}
	w.push(step{kind: numberControlStep, host: current.host, span: current.span, number: current.number, exprs: current.exprs, terms: current.terms, index: current.index})
	return nil
}

func (w *Writer) beginGenericFor(stmt *ast.GenericForStmt, host keyspace.Term, span source.Span) error {
	if stmt == nil || len(stmt.Names) == 0 || len(stmt.Exprs) == 0 {
		return fmt.Errorf("lualower: invalid generic for statement")
	}
	if host == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: generic for began outside its host Body")
	}
	w.push(step{kind: finishGenericControlsStep, host: host, span: span, generic: stmt})
	return w.valueList(stmt.Exprs, host, span)
}

func (w *Writer) finishGenericControls(current step) error {
	stmt := current.generic
	if stmt == nil || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: invalid generic for continuation")
	}
	control, _ := w.phases.Result()
	return w.openLoopBody(stmt.Stmts, w.chunkSpan(stmt.Stmts), step{
		kind: finishLoopStep, host: current.host, span: current.span, generic: stmt,
		control: control, cellMark: w.CellMark(),
	})
}

func (w *Writer) openLoopBody(stmts []ast.Stmt, span source.Span, next step) error {
	if next.host == 0 || w.scopes.Owner() != next.host {
		return fmt.Errorf("lualower: loop body opened outside its host Body")
	}
	body, err := w.scopes.EnterBlock(span)
	if err != nil {
		return fmt.Errorf("lualower: could not create loop Body: %w", err)
	}
	next.body = body
	if err := w.declareLoopCells(next); err != nil {
		return err
	}
	return w.scheduleBody(stmts, span, next)
}

func (w *Writer) declareLoopCells(next step) error {
	if next.body == 0 || w.scopes.Owner() != next.body {
		return fmt.Errorf("lualower: loop Cells declared outside their Body")
	}
	switch {
	case next.while != nil, next.repeat != nil:
		return nil
	case next.number != nil:
		loop := next.number
		id, ok := w.binding.NumForSymbol(loop)
		if !ok || id == 0 {
			return fmt.Errorf("lualower: binder has no numeric for symbol")
		}
		cell, err := w.scopes.DeclareLoop(id, w.positionSpan(loop.NamePosition))
		if err != nil {
			return err
		}
		return w.RememberCell(cell)
	case next.generic != nil:
		loop := next.generic
		ids := w.binding.GenericForSymbols(loop)
		if len(ids) != len(loop.Names) {
			return fmt.Errorf("lualower: binder has incomplete generic for symbols")
		}
		for index, id := range ids {
			if id == 0 {
				return fmt.Errorf("lualower: binder has zero generic for symbol %d", index)
			}
			span := w.span(loop)
			if index < len(loop.NamePositions) {
				span = w.positionSpan(loop.NamePositions[index])
			}
			cell, err := w.scopes.DeclareLoop(id, span)
			if err != nil {
				return err
			}
			if err := w.RememberCell(cell); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("lualower: loop continuation has no declaration form")
	}
	return nil
}

func (w *Writer) finishLoop(current step) error {
	body, _ := w.phases.Result()
	if body == 0 || body != current.body || current.host == 0 || w.scopes.Owner() != current.host {
		return fmt.Errorf("lualower: mismatched loop Body")
	}
	var kind flowkind.LoopKind
	switch {
	case current.while != nil:
		kind = flowkind.LoopWhile
	case current.repeat != nil:
		kind = flowkind.LoopRepeat
	case current.number != nil:
		kind = flowkind.LoopNumericFor
	case current.generic != nil:
		kind = flowkind.LoopGenericFor
	default:
		return fmt.Errorf("lualower: loop continuation has no form")
	}
	term, err := w.Loop(current.span, current.host, body, current.control, current.cellMark, kind)
	if err != nil {
		return err
	}
	return w.appendTo(current.host, term)
}

func (w *Writer) openBody(stmts []ast.Stmt, span source.Span, next step) error {
	if next.host == 0 || w.scopes.Owner() != next.host {
		return fmt.Errorf("lualower: branch Body opened outside its host Body")
	}
	body, err := w.scopes.EnterBlock(span)
	if err != nil {
		return fmt.Errorf("lualower: could not create branch Body: %w", err)
	}
	next.body = body
	return w.scheduleBody(stmts, span, next)
}

func (w *Writer) scheduleBody(stmts []ast.Stmt, span source.Span, next step) error {
	// The body inbox retains the concrete child Body and resolved span. The
	// source runner owns the three body phases; control never recovers either
	// value from whichever lexical Body happens to be active later.
	w.push(next)
	if err := w.bodies.PushClose(next.body, span); err != nil {
		return err
	}
	if err := w.bodies.PushStatements(stmts, 0, next.body, span); err != nil {
		return err
	}
	return w.bodies.PushPrepare(stmts, next.body, span)
}

func (w *Writer) expression(expr ast.Expr, host keyspace.Term, span source.Span) error {
	if expr == nil || host == 0 || span.File == "" {
		return fmt.Errorf("lualower: absent control expression")
	}
	return w.expressions.Push(expr, host, span)
}

func (w *Writer) appendTo(host, term keyspace.Term) error {
	if host == 0 || term == 0 || w.scopes.Owner() != host {
		return fmt.Errorf("lualower: control result has no active host Body")
	}
	return w.scopes.Append(term)
}

func (w *Writer) valueList(exprs []ast.Expr, host keyspace.Term, span source.Span) error {
	if host == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid control Values host")
	}
	return w.values.ScheduleValues(exprs, host, span)
}

func (w *Writer) push(next step) {
	w.steps = append(w.steps, next)
	w.phases.Push(continuation.Control)
}

func (w *Writer) ready() error {
	if w == nil || !w.constructed || w.phases == nil || w.binding == nil ||
		w.scopes == nil || w.values == nil || w.expressions == nil || w.bodies == nil {
		return fmt.Errorf("lualower: incomplete control authority")
	}
	return nil
}

func indexIssues(binding *bind.Result) map[ast.Stmt]bind.ControlIssue {
	if binding == nil {
		return nil
	}
	issues := binding.ControlIssues()
	if len(issues) == 0 {
		return nil
	}
	indexed := make(map[ast.Stmt]bind.ControlIssue, len(issues))
	for _, issue := range issues {
		var stmt ast.Stmt
		switch issue.Kind {
		case bind.ControlIssueDuplicateLabel:
			stmt = issue.Label
		case bind.ControlIssueUndefinedLabel, bind.ControlIssueGotoEntersLocal:
			stmt = issue.Goto
		case bind.ControlIssueBreakOutsideLoop:
			stmt = issue.Break
		}
		if stmt != nil {
			indexed[stmt] = issue
		}
	}
	return indexed
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
