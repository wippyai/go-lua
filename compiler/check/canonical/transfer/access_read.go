package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

type accessValueReadQuery struct {
	Expr                 *ast.AttrGetExpr
	ReadExpr             func(ast.Expr) (product.AbstractValue, bool)
	StaticPath           func(ast.Expr) (constraint.Path, bool)
	StaticMember         func(*ast.AttrGetExpr) (value.MemberKey, bool)
	AllowGradualFallback bool
}

// readAccessValue is the canonical value-read reducer for field/index access.
// Callers lower syntax and policy at the boundary; the read law itself is shared:
// point-state static facts, product member/index read, callable overlay, dynamic
// writeback admission, then index-presence/length refinement.
func (t *Transfer) readAccessValue(out *flow.PointState, q accessValueReadQuery) (product.AbstractValue, bool) {
	if q.Expr == nil || q.ReadExpr == nil {
		return product.AbstractValue{}, false
	}
	staticPath := q.StaticPath
	if staticPath == nil {
		staticPath = t.staticPathOfExpr
	}
	staticMember := q.StaticMember
	if staticMember == nil {
		staticMember = staticMemberKey
	}

	path, hasPath := staticPath(q.Expr)
	if out != nil && hasPath {
		if fact := flow.PointFactsOf(*out).ReadStaticMemberValue(path, t.pointReadPolicy(out)); fact.State == flow.StateResolved {
			return fact.Value, true
		}
	}

	base, ok := q.ReadExpr(q.Expr.Object)
	if !ok || base.IsZero() {
		if q.AllowGradualFallback && t.gradualAnySource(out, q.Expr) {
			return product.GradualAny(), true
		}
		return product.AbstractValue{}, false
	}

	if member, isStatic := staticMember(q.Expr); isStatic {
		read, ok := product.RuntimeMemberOf(base, member)
		if !ok || read.IsZero() {
			if cv, ok := t.readKnownCallableAccess(out, path, hasPath, read); ok {
				return cv, true
			}
			if q.AllowGradualFallback && t.gradualAnySource(out, q.Expr) {
				return product.GradualAny(), true
			}
			return product.AbstractValue{}, false
		}
		if cv, ok := t.readKnownCallableAccess(out, path, hasPath, read); ok {
			return cv, true
		}
		return t.refineIndexRead(out, q.Expr, base, read), true
	}

	key, ok := q.ReadExpr(q.Expr.Key)
	if !ok || key.IsZero() {
		return product.AbstractValue{}, false
	}
	read, ok := product.RuntimeIndexOf(base, key)
	if !ok || read.IsZero() {
		if admitted, admittedOK := t.refineByIndexWriteAdmission(out, q.Expr); admittedOK {
			return admitted, true
		}
		return product.AbstractValue{}, false
	}
	return t.refineIndexRead(out, q.Expr, base, read), true
}

func (t *Transfer) readKnownCallableAccess(out *flow.PointState, path constraint.Path, hasPath bool, read product.AbstractValue) (product.AbstractValue, bool) {
	if out == nil || !hasPath {
		return product.AbstractValue{}, false
	}
	cv := flow.PointFactsOf(*out).ReadKnownCallablePath(path, read, t.pointReadPolicy(out))
	if cv.State != flow.StateResolved {
		return product.AbstractValue{}, false
	}
	return cv.Value, true
}
