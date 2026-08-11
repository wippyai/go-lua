// Package inbox owns the narrow typed crossings between lowering verticals.
// Payloads never enter phase.Stack: only the corresponding closed owner token
// does, so there is no second instruction or routing vocabulary.
package inbox

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
	"github.com/wippyai/go-lua/program/source"
)

// Expression is one ordinary source expression awaiting dispatch to its sole
// semantic owner.
type Expression struct {
	Expr ast.Expr
	Host keyspace.Term
	Span source.Span
}

// Expressions is the source expression inbox. Its LIFO payload stack mirrors
// the owner tokens pushed onto phase.Stack.
type Expressions struct {
	phases  *phase.Stack
	pending []Expression
}

func NewExpressions(phases *phase.Stack) *Expressions {
	return &Expressions{phases: phases}
}

func (q *Expressions) Push(expr ast.Expr, host keyspace.Term, span source.Span) error {
	exact, ok := ExpressionSpan(expr, span.File)
	if q == nil || q.phases == nil || !ok || host == 0 || span.File == "" || span != exact {
		return fmt.Errorf("programlower: invalid pending expression")
	}
	q.pending = append(q.pending, Expression{Expr: expr, Host: host, Span: span})
	q.phases.Push(phase.SyntaxExpression)
	return nil
}

// ExpressionSpan returns the exact structural source extent of one concrete
// expression. It is the single typed-nil-safe boundary check used by inbox
// producers; it classifies no semantic owner.
func ExpressionSpan(expr ast.Expr, file string) (source.Span, bool) {
	var holder ast.PositionHolder
	switch node := expr.(type) {
	case *ast.TrueExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.FalseExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.NilExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.NumberExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.StringExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.Comma3Expr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.IdentExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.AttrGetExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.TableExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.FuncCallExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.LogicalOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.RelationalOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.StringConcatOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.ArithmeticOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.UnaryMinusOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.UnaryNotOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.UnaryLenOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.UnaryBNotOpExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.FunctionExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.CastExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.NonNilAssertExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	default:
		return source.Span{}, false
	}
	if file == "" {
		return source.Span{}, false
	}
	span, ok := sourcecoord.Build(file, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return source.Span{}, false
	}
	return span, true
}

func (q *Expressions) Pop() (Expression, error) {
	if q == nil || len(q.pending) == 0 {
		return Expression{}, fmt.Errorf("programlower: expression token has no payload")
	}
	last := len(q.pending) - 1
	request := q.pending[last]
	q.pending = q.pending[:last]
	return request, nil
}

func (q *Expressions) Clean() bool {
	return q != nil && len(q.pending) == 0
}
