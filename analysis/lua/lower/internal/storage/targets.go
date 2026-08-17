package storage

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// TargetMark starts one delayed assignment-target range.
func (w *Writer) TargetMark() TargetMark {
	return TargetMark{owner: w, target: len(w.targets)}
}

// RememberTarget retains one evaluated target in source order.
func (w *Writer) RememberTarget(span source.Span, target keyspace.Term) error {
	if w == nil || target == 0 {
		return fmt.Errorf("lualower: invalid assignment target")
	}
	w.targets = append(w.targets, target)
	w.targetSpans = append(w.targetSpans, span)
	return nil
}

// Assign commits one delayed target group, then records its additive static
// publication metadata before its lexical Body observes the statement. assign
// is nil only for a Function-owned definition commit, whose target is already
// exact and which has no assignment-publication syntax. Import facts remain
// candidates; Rules later test the abstract callee value at each Call.
func (w *Writer) Assign(
	span source.Span,
	owner keyspace.Term,
	mark TargetMark,
	values keyspace.Term,
	assign *ast.AssignStmt,
) (keyspace.Term, error) {
	if w == nil || w.collector == nil || w.static == nil ||
		mark.owner != w || mark.target < 0 || mark.target > len(w.targets) {
		return 0, fmt.Errorf("lualower: invalid assignment target mark")
	}
	if assign != nil && len(assign.Lhs) != len(w.targets)-mark.target {
		return 0, fmt.Errorf("lualower: assignment source targets disagree with evaluated targets")
	}
	targets := w.targets[mark.target:]
	targetSpans := make([]source.Span, len(w.targetSpans)-mark.target)
	for index, targetSpan := range w.targetSpans[mark.target:] {
		targetSpans[index] = targetSpan
	}
	term := w.collector.Assign(
		span, owner, targets, targetSpans, values,
	)
	w.targets = w.targets[:mark.target]
	w.targetSpans = w.targetSpans[:mark.target]
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not lower assignment")
	}
	if assign != nil {
		if err := w.static.PublishTypePublications(assign, term); err != nil {
			return 0, err
		}
	}
	return term, nil
}

// ScheduleTarget queues one assignment-target expression. It never creates a
// Read; the resulting Cell or Lens is retained only as an address.
func (w *Writer) ScheduleTarget(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if w == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid assignment target")
	}
	switch node := expr.(type) {
	case *ast.IdentExpr:
		if node == nil {
			return fmt.Errorf("lualower: absent assignment identifier target")
		}
		w.schedule(step{kind: stepTarget, expr: node, owner: owner, span: span})
		return nil
	case *ast.AttrGetExpr:
		if node == nil {
			return fmt.Errorf("lualower: absent assignment attribute target")
		}
		w.schedule(step{kind: stepTarget, expr: node, owner: owner, span: span})
		return nil
	default:
		return fmt.Errorf("lualower: expression %T is not an assignment target", expr)
	}
}

// ScheduleAssignment queues exact left-to-right target address evaluation followed
// by one ordinary Values route for the right-hand expression list.
func (w *Writer) ScheduleAssignment(assign *ast.AssignStmt, owner keyspace.Term, span source.Span) error {
	if w == nil || assign == nil || len(assign.Lhs) == 0 || owner == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid assignment")
	}
	targetSpans := make([]source.Span, len(assign.Lhs))
	for index, target := range assign.Lhs {
		switch node := target.(type) {
		case *ast.IdentExpr:
			if node == nil {
				return fmt.Errorf("lualower: absent assignment identifier target")
			}
		case *ast.AttrGetExpr:
			if node == nil {
				return fmt.Errorf("lualower: absent assignment attribute target")
			}
		default:
			return fmt.Errorf("lualower: expression %T is not an assignment target", target)
		}
		targetSpans[index] = w.span(target)
	}
	w.schedule(step{
		kind:        stepTargets,
		assign:      assign,
		owner:       owner,
		span:        span,
		targetSpans: targetSpans,
		mark:        w.TargetMark(),
	})
	return nil
}

func (w *Writer) runTarget(current step) error {
	switch expr := current.expr.(type) {
	case *ast.IdentExpr:
		if expr == nil {
			return fmt.Errorf("lualower: absent assignment identifier target")
		}
		term, err := w.resolveIdentifier(expr, current.owner, current.span, false)
		if err != nil {
			return err
		}
		w.stack.SetResult(term, false)
		return nil
	case *ast.AttrGetExpr:
		if expr == nil {
			return fmt.Errorf("lualower: absent assignment attribute target")
		}
		return w.beginLens(expr, current.owner, current.span, false)
	default:
		return fmt.Errorf("lualower: invalid assignment target %T", current.expr)
	}
}

func (w *Writer) runTargets(current step) error {
	if current.assign == nil || current.index < 0 || current.index > len(current.assign.Lhs) {
		return fmt.Errorf("lualower: invalid assignment-target cursor")
	}
	if current.index == len(current.assign.Lhs) {
		if w.values == nil {
			return fmt.Errorf("lualower: missing Values authority")
		}
		w.schedule(step{
			kind: stepFinishAssign, assign: current.assign, owner: current.owner,
			span: current.span, mark: current.mark,
		})
		// A one-target assignment consumes a scalar result. Keep a final open
		// producer as the authored fixed member so Assign's Values relation
		// preserves the exact RHS occurrence while wider assignments retain the
		// open tail for Lua adjustment.
		if len(current.assign.Lhs) == 1 {
			return w.values.ScheduleScalarValues(current.assign.Rhs, current.owner, current.span)
		}
		return w.values.ScheduleValues(current.assign.Rhs, current.owner, current.span)
	}
	expr := current.assign.Lhs[current.index]
	if expr == nil {
		return fmt.Errorf("lualower: absent assignment target")
	}
	current.index++
	span := current.targetSpans[current.index-1]
	w.schedule(step{kind: stepTargets, assign: current.assign, index: current.index, owner: current.owner, span: current.span, targetSpans: current.targetSpans, mark: current.mark})
	w.schedule(step{kind: stepAppendTarget, expr: expr, owner: current.owner, span: span})
	return w.scheduleTargetChild(expr, current.owner, span)
}

func (w *Writer) scheduleTargetChild(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	return w.ScheduleTarget(expr, owner, span)
}
