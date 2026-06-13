package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) typeCastPostconditionRefinement(fact semantics.CallFact) (factflow.PostconditionRefinement, bool) {
	t, argPath, ok := l.directTypeCastCall(fact)
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(argPath, factflow.NewValueConstraint(l.typeWitnessValue(t))), true
}

func (l *lowerer) typeCastCallResultValue(fact semantics.CallFact) (factflow.CallResultValue, bool) {
	t, _, ok := l.directTypeCastCall(fact)
	if !ok {
		return factflow.CallResultValue{}, false
	}
	return factflow.NewCallResultValue(0, l.typeWitnessValue(t)), true
}

func (l *lowerer) directTypeCastCall(fact semantics.CallFact) (typ.Type, path.Path, bool) {
	call, ok := branchcond.TypeCall(fact.Call)
	if !ok {
		return nil, path.Path{}, false
	}
	t, ok := l.typeValueExpr(fact.Func)
	if !ok {
		return nil, path.Path{}, false
	}
	argPath, ok := pathexpr.Resolve(call.Args[0], l.bindings)
	if !ok || argPath.IsEmpty() {
		return nil, path.Path{}, false
	}
	return t, argPath, true
}

func (l *lowerer) typeValueExpr(expr ast.Expr) (typ.Type, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || l.bindings == nil {
		return nil, false
	}
	decl, ok := l.bindings.TypeValueRef(ident)
	if !ok {
		return nil, false
	}
	return typeresolve.New(l.bindings).Decl(decl)
}
