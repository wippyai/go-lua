package control

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
