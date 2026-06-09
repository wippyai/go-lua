package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/literal"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// projectExprValueResolver exposes only already-known point-state facts to
// product call providers. It deliberately does not evaluate call expressions:
// providers are part of the current call's transfer, so re-entering evalCall from
// a fallback resolver creates a transfer-local recursion outside the Kildall/db
// summary fixed point.
func (t *Transfer) projectExprValueResolver(
	out *flow.PointState,
) func(ast.Expr) (product.AbstractValue, bool) {
	return func(e ast.Expr) (product.AbstractValue, bool) {
		return t.projectExprValue(out, e)
	}
}

func (t *Transfer) exprTypeResolver(exprValue func(ast.Expr) (product.AbstractValue, bool)) func(ast.Expr) typ.Type {
	return func(e ast.Expr) typ.Type {
		if av, ok := exprValue(e); ok && !av.IsZero() {
			if pt := av.ProjectValue(); pt != nil && !typ.IsUnknown(pt) {
				return pt
			}
		}
		return typ.Unknown
	}
}

func (t *Transfer) exprValueResolver(
	out *flow.PointState,
	demand func(int, paramevidence.ParamContract),
) func(ast.Expr) (product.AbstractValue, bool) {
	return func(e ast.Expr) (product.AbstractValue, bool) {
		return t.resolveExprValue(out, e, demand)
	}
}

// resolveExprType resolves an expression's value type against the live Env for
// callee, receiver, and iterator-source resolution. It projects determined
// product values and falls back to gradual any only for genuinely gradual
// expression sources.
func (t *Transfer) resolveExprType(
	out *flow.PointState,
	e ast.Expr,
	demand func(int, paramevidence.ParamContract),
) typ.Type {
	if e == nil {
		return typ.Unknown
	}
	return t.exprTypeResolver(t.exprValueResolver(out, demand))(e)
}

func (t *Transfer) projectExprTypeResolver(out *flow.PointState) func(ast.Expr) typ.Type {
	return t.exprTypeResolver(t.projectExprValueResolver(out))
}

func (t *Transfer) projectExprValue(out *flow.PointState, expr ast.Expr) (product.AbstractValue, bool) {
	switch e := expr.(type) {
	case nil:
		return product.AbstractValue{}, false
	case *ast.IdentExpr:
		return t.projectIdentValue(out, e)
	case *ast.NilExpr:
		return product.FromType(typ.Nil), true
	case *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr, *ast.FalseExpr:
		if lit, ok := literal.FromExpr(expr); ok {
			return product.FromType(lit), true
		}
		return product.AbstractValue{}, false
	case *ast.FunctionExpr:
		if t.funcTyper != nil {
			if fn := t.funcTyper.FuncType(e); fn != nil {
				return product.FromType(fn), true
			}
		}
		return product.AbstractValue{}, false
	case *ast.CastExpr:
		if t.castType != nil && e.Type != nil {
			if ct := t.castType(e.Type); ct != nil && !typ.IsAbsentOrUnknown(ct) {
				return product.FromType(ct), true
			}
		}
		return t.projectExprValue(out, e.Expr)
	case *ast.AttrGetExpr:
		return t.projectAttrGetValue(out, e)
	case *ast.UnaryLenOpExpr:
		if _, ok := t.projectExprValue(out, e.Expr); ok {
			return product.FromType(typ.Integer), true
		}
		return product.AbstractValue{}, false
	case *ast.FuncCallExpr:
		return t.projectDynamicCallValue(out, e)
	default:
		if t.gradualAnySource(out, expr) {
			return product.GradualAny(), true
		}
		return product.AbstractValue{}, false
	}
}

func (t *Transfer) projectDynamicCallValue(out *flow.PointState, call *ast.FuncCallExpr) (product.AbstractValue, bool) {
	if call == nil {
		return product.AbstractValue{}, false
	}
	subject := call.Func
	if call.Method != "" {
		subject = call.Receiver
	}
	av, ok := t.projectExprValue(out, subject)
	if !ok || av.IsZero() {
		return product.AbstractValue{}, false
	}
	if av.IsGradualTop() {
		return product.GradualAny(), true
	}
	if typ.IsAny(av.ProjectValue()) {
		return product.FromType(typ.Any), true
	}
	return product.AbstractValue{}, false
}

func (t *Transfer) projectIdentValue(out *flow.PointState, e *ast.IdentExpr) (product.AbstractValue, bool) {
	return t.readIdentValue(out, identValueReadQuery{
		Expr:                     e,
		AllowGradualTopAdmission: true,
	})
}

func (t *Transfer) projectAttrGetValue(out *flow.PointState, e *ast.AttrGetExpr) (product.AbstractValue, bool) {
	return t.readAccessValue(out, accessValueReadQuery{
		Expr: e,
		ReadExpr: func(expr ast.Expr) (product.AbstractValue, bool) {
			return t.projectExprValue(out, expr)
		},
		AllowGradualTopAdmission: true,
	})
}
