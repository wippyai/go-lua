package storage

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ScheduleExpression queues one source-dispatched storage expression. The
// dispatcher supplies its already resolved span so no delayed step recovers
// position data from an active source context.
func (w *Writer) ScheduleExpression(expr ast.Expr, owner keyspace.Term, span source.Span) error {
	if w == nil || owner == 0 || span.File == "" {
		return fmt.Errorf("lualower: invalid storage expression")
	}
	switch node := expr.(type) {
	case *ast.IdentExpr:
		if node == nil {
			return fmt.Errorf("lualower: absent storage identifier")
		}
		w.schedule(step{kind: stepExpression, expr: node, owner: owner, span: span})
		return nil
	case *ast.AttrGetExpr:
		if node == nil {
			return fmt.Errorf("lualower: absent storage attribute")
		}
		w.schedule(step{kind: stepExpression, expr: node, owner: owner, span: span})
		return nil
	default:
		return fmt.Errorf("lualower: expression %T is not storage-owned", expr)
	}
}

func (w *Writer) runExpression(current step) error {
	switch expr := current.expr.(type) {
	case *ast.IdentExpr:
		if expr == nil {
			return fmt.Errorf("lualower: absent storage identifier")
		}
		term, err := w.resolveIdentifier(expr, current.owner, current.span, true)
		if err != nil {
			return err
		}
		w.stack.SetResult(term, false)
		return nil
	case *ast.AttrGetExpr:
		if expr == nil {
			return fmt.Errorf("lualower: absent storage attribute")
		}
		return w.beginLens(expr, current.owner, current.span, true)
	default:
		return fmt.Errorf("lualower: invalid storage expression %T", current.expr)
	}
}

func (w *Writer) resolveIdentifier(expr *ast.IdentExpr, owner keyspace.Term, span source.Span, read bool) (keyspace.Term, error) {
	cell, err := w.ResolveCell(expr)
	if err != nil || !read {
		return cell, err
	}
	if w.static == nil {
		return 0, fmt.Errorf("lualower: missing static authority")
	}
	if w.binding.IsImplicitGlobalUse(expr) && w.static.StaticDepth() == 0 {
		return w.implicitRead(span, owner, cell)
	}
	return w.Read(span, owner, cell)
}
