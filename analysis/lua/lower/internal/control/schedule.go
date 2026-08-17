package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
