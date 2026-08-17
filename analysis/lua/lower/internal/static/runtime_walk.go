package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) push(next walkStep) {
	w.steps = append(w.steps, next)
	w.phases.Push(continuation.Static)
}

func (w *Writer) expression(expr ast.Expr, body keyspace.Term, span source.Span) error {
	if !validRuntimeExpr(expr) || body == 0 || span.File == "" {
		return fmt.Errorf("lualower: absent static expression operand")
	}
	return w.expressions.Push(expr, body, span)
}

func (w *Writer) values(exprs []ast.Expr, body keyspace.Term, span source.Span) error {
	return w.evaluations.ScheduleValues(exprs, body, span)
}

func (w *Writer) result() keyspace.Term {
	term, _ := w.phases.Result()
	return term
}

func (w *Writer) publish(term keyspace.Term, err error) error {
	if err != nil {
		return err
	}
	w.phases.SetResult(term, false)
	return nil
}

func (w *Writer) ready() error {
	if w == nil || w.binding == nil || w.scopes == nil || w.phases == nil ||
		w.expressions == nil || w.evaluations == nil {
		return fmt.Errorf("lualower: static writer is not scheduled")
	}
	return nil
}

func (w *Writer) typeSpan(expr ast.TypeExpr) source.Span {
	if !validTypeExpr(expr) {
		return coord.Invalid(w.sourceName)
	}
	return w.span(expr)
}

func (w *Writer) annotationsSpan(annotations []ast.AnnotationExpr, fallback source.Span) source.Span {
	if len(annotations) == 0 {
		return fallback
	}
	return w.span(&annotations[0])
}

func (w *Writer) expressionSpan(expr ast.Expr) source.Span {
	if !validRuntimeExpr(expr) {
		return coord.Invalid(w.sourceName)
	}
	return w.span(expr)
}

func validTypeExpr(expr ast.TypeExpr) bool {
	switch node := expr.(type) {
	case *ast.AnnotatedTypeExpr:
		return node != nil && node.Inner != nil
	case *ast.PrimitiveTypeExpr:
		return node != nil
	case *ast.OptionalTypeExpr:
		return node != nil
	case *ast.UnionTypeExpr:
		return node != nil
	case *ast.IntersectionTypeExpr:
		return node != nil
	case *ast.ArrayTypeExpr:
		return node != nil
	case *ast.MapTypeExpr:
		return node != nil
	case *ast.RecordTypeExpr:
		return node != nil
	case *ast.FunctionTypeExpr:
		return node != nil
	case *ast.AssertsTypeExpr:
		return node != nil
	case *ast.TypeRefExpr:
		return node != nil
	case *ast.GenericTypeExpr:
		return node != nil
	case *ast.LiteralTypeExpr:
		return node != nil
	case *ast.TypeOfExpr:
		return node != nil
	case *ast.KeyOfExpr:
		return node != nil
	case *ast.IndexAccessExpr:
		return node != nil
	case *ast.ConditionalTypeExpr:
		return node != nil
	default:
		return false
	}
}

func validRuntimeExpr(expr ast.Expr) bool {
	switch node := expr.(type) {
	case *ast.TrueExpr:
		return node != nil
	case *ast.FalseExpr:
		return node != nil
	case *ast.NilExpr:
		return node != nil
	case *ast.NumberExpr:
		return node != nil
	case *ast.StringExpr:
		return node != nil
	case *ast.Comma3Expr:
		return node != nil
	case *ast.IdentExpr:
		return node != nil
	case *ast.AttrGetExpr:
		return node != nil
	case *ast.TableExpr:
		return node != nil
	case *ast.FuncCallExpr:
		return node != nil
	case *ast.LogicalOpExpr:
		return node != nil
	case *ast.RelationalOpExpr:
		return node != nil
	case *ast.StringConcatOpExpr:
		return node != nil
	case *ast.ArithmeticOpExpr:
		return node != nil
	case *ast.UnaryMinusOpExpr:
		return node != nil
	case *ast.UnaryNotOpExpr:
		return node != nil
	case *ast.UnaryLenOpExpr:
		return node != nil
	case *ast.UnaryBNotOpExpr:
		return node != nil
	case *ast.FunctionExpr:
		return node != nil
	case *ast.CastExpr:
		return node != nil
	case *ast.NonNilAssertExpr:
		return node != nil
	default:
		return false
	}
}
