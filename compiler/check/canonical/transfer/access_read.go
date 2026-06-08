package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
)

type identValueReadQuery struct {
	Expr                 *ast.IdentExpr
	AllowGradualFallback bool
}

type accessValueReadQuery struct {
	Expr                 *ast.AttrGetExpr
	ReadExpr             func(ast.Expr) (product.AbstractValue, bool)
	StaticPath           func(ast.Expr) (constraint.Path, bool)
	StaticMember         func(*ast.AttrGetExpr) (value.MemberKey, bool)
	AllowGradualFallback bool
}

// readIdentValue is the canonical transfer read for a root identifier. It keeps
// symbol storage, callable identity overlays, type-value bindings, and optional
// gradual reads in one law shared by evaluation and product-call projection.
func (t *Transfer) readIdentValue(out *flow.PointState, q identValueReadQuery) (product.AbstractValue, bool) {
	if q.Expr == nil {
		return product.AbstractValue{}, false
	}
	sym := t.symbolOf(q.Expr)
	if sym == 0 {
		if meta, ok := t.typeValueOf(q.Expr); ok {
			return meta, true
		}
		return product.AbstractValue{}, false
	}
	av, ok := t.symbolValue(out, sym)
	path := constraint.NewPath(sym, "")
	if !ok || av.IsZero() {
		if out != nil {
			if cv := flow.PointFactsOfBorrowed(out).ReadCallablePathValue(path, t.pointReadPolicy(out)); cv.State == flow.StateResolved {
				return cv.Value, true
			}
		}
		if meta, ok := t.typeValueOf(q.Expr); ok {
			return meta, true
		}
		if q.AllowGradualFallback && t.unannotatedParam.Contains(sym) {
			return product.GradualAny(), true
		}
		return product.AbstractValue{}, false
	}
	if pt := av.ProjectValue(); pt != nil && pt.Kind() == kind.Function {
		if out != nil {
			if cv := flow.PointFactsOfBorrowed(out).ReadCallablePath(path, av, t.pointReadPolicy(out)); cv.State == flow.StateResolved {
				return cv.Value, true
			}
		}
	}
	return av, true
}

// readAccessValue is the canonical value-read reducer for field/index access.
// Callers lower syntax and policy at the boundary; the read law itself is shared:
// point-state static facts, product member/index read, callable overlay, dynamic
// writeback admission, then index-presence/length refinement.
func (t *Transfer) readAccessValue(out *flow.PointState, q accessValueReadQuery) (product.AbstractValue, bool) {
	if q.Expr == nil || q.ReadExpr == nil {
		return product.AbstractValue{}, false
	}
	facts := flow.PointFactsOf(flow.PointState{})
	if out != nil {
		facts = flow.PointFactsOfBorrowed(out)
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
	base, ok := q.ReadExpr(q.Expr.Object)
	if !ok || base.IsZero() {
		if out != nil {
			read := facts.ReadAccess(flow.AccessReadQuery{
				Kind:    flow.AccessReadStaticMember,
				Path:    path,
				HasPath: hasPath,
				Policy:  t.pointReadPolicy(out),
			})
			if read.State == flow.StateResolved {
				return read.Value, true
			}
		}
		if q.AllowGradualFallback && t.gradualAnySource(out, q.Expr) {
			return product.GradualAny(), true
		}
		return product.AbstractValue{}, false
	}

	if member, isStatic := staticMember(q.Expr); isStatic {
		read := facts.ReadAccess(flow.AccessReadQuery{
			Kind:    flow.AccessReadStaticMember,
			Path:    path,
			HasPath: hasPath,
			Base:    base,
			Member:  member,
			Policy:  t.pointReadPolicy(out),
		})
		if read.State != flow.StateResolved || read.Value.IsZero() {
			if q.AllowGradualFallback && t.gradualAnySource(out, q.Expr) {
				return product.GradualAny(), true
			}
			return product.AbstractValue{}, false
		}
		return t.refineIndexRead(out, q.Expr, base, read.Value, product.AbstractValue{}), true
	}

	key, ok := q.ReadExpr(q.Expr.Key)
	if !ok || key.IsZero() {
		return product.AbstractValue{}, false
	}
	read := facts.ReadAccess(flow.AccessReadQuery{
		Kind: flow.AccessReadDynamicIndex,
		Base: base,
		Key:  key,
	})
	if read.State != flow.StateResolved || read.Value.IsZero() {
		if out != nil {
			refined := facts.RefineIndexRead(t.indexReadRefinementQuery(out, q.Expr, base, product.AbstractValue{}, key))
			if refined.State == flow.StateResolved {
				return refined.Value, true
			}
		}
		return product.AbstractValue{}, false
	}
	return t.refineIndexRead(out, q.Expr, base, read.Value, key), true
}
