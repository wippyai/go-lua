// Package functiontype lowers bound Lua function literals to checker function
// types. It is syntax/binding metadata only; value facts are derived by the
// transfer interpreter.
package functiontype

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ValueExpression(fn *ast.FunctionExpr, bindings *bind.Result, resolver *typeresolve.Resolver) (typ.Type, bool) {
	if t, ok := ContextualReturnExpression(fn, bindings, resolver); ok {
		return t, true
	}
	return Expression(fn, bindings, resolver)
}

func ContextualReturnExpression(fn *ast.FunctionExpr, bindings *bind.Result, resolver *typeresolve.Resolver) (typ.Type, bool) {
	if fn == nil || bindings == nil || resolver == nil {
		return nil, false
	}
	parent, ok := bindings.ParentFunction(fn)
	if !ok || parent == nil {
		return nil, false
	}
	idx, ok := returnExpressionIndex(parent.Stmts, fn)
	if !ok {
		return nil, false
	}
	declared := ReturnTypeExprs(parent.ReturnTypes)
	if idx >= len(declared) || declared[idx] == nil {
		return nil, false
	}
	t, ok := resolver.Type(declared[idx])
	if !ok || t == nil {
		return nil, false
	}
	fnType, ok := typecall.ContextualCallable(t)
	if !ok || fnType == nil {
		return nil, false
	}
	return fnType, true
}

func Expression(fn *ast.FunctionExpr, bindings *bind.Result, resolver *typeresolve.Resolver) (typ.Type, bool) {
	if fn == nil || bindings == nil || resolver == nil {
		return nil, false
	}
	return FromBindings(fn, bindings, resolver.Type, resolver.Decl)
}

func FromBindings(
	fn *ast.FunctionExpr,
	bindings *bind.Result,
	resolveType func(ast.TypeExpr) (typ.Type, bool),
	resolveDecl func(bind.TypeDecl) (typ.Type, bool),
) (typ.Type, bool) {
	if fn == nil || bindings == nil || resolveType == nil || resolveDecl == nil {
		return nil, false
	}
	builder := typ.Func()
	for _, decl := range bindings.FunctionTypeParams(fn) {
		t, ok := resolveDecl(decl)
		param, paramOK := t.(*typ.TypeParam)
		if !ok || !paramOK || param == nil {
			return nil, false
		}
		builder.TypeParamRef(param)
	}
	slots := bindings.ParamSlots(fn)
	if hasUntypedRegularParam(slots) {
		builder.Variadic(typ.Any)
	} else {
		builder.ReserveParams(len(slots))
		for _, slot := range slots {
			t := typ.Type(nil)
			if slot.Type != nil {
				resolved, ok := resolveType(slot.Type)
				if !ok {
					return nil, false
				}
				t = resolved
			} else if slot.ImplicitSelf {
				t = implicitSelf(fn, bindings, resolveDecl)
			} else {
				t = typ.Any
			}
			if slot.Vararg {
				builder.Variadic(t)
				continue
			}
			builder.Param(slot.Name, t)
		}
	}
	returns := make([]typ.Type, 0, len(fn.ReturnTypes))
	for _, ret := range ReturnTypeExprs(fn.ReturnTypes) {
		t, ok := resolveType(ret)
		if !ok {
			return nil, false
		}
		returns = append(returns, t)
	}
	if len(returns) != 0 {
		builder.Returns(returns...)
	}
	return builder.Build(), true
}

// hasUntypedRegularParam reports whether any fixed (non-vararg) parameter
// lacks a type annotation. The trailing `...` slot is excluded: an untyped
// variadic tail widens only to Variadic(Any) and must not force the fixed
// typed prefix to collapse.
func hasUntypedRegularParam(slots []bind.ParamSlot) bool {
	for _, slot := range slots {
		if slot.Vararg {
			continue
		}
		if slot.Type == nil && !slot.ImplicitSelf {
			return true
		}
	}
	return false
}

func implicitSelf(fn *ast.FunctionExpr, bindings *bind.Result, resolveDecl func(bind.TypeDecl) (typ.Type, bool)) typ.Type {
	if bindings == nil || resolveDecl == nil {
		return typ.Any
	}
	decl, ok := bindings.MethodReceiverType(fn)
	if !ok {
		return typ.Any
	}
	t, ok := resolveDecl(decl)
	if !ok || t == nil || typ.IsNever(t) {
		return typ.Any
	}
	return t
}

func ReturnTypeExprs(types []ast.TypeExpr) []ast.TypeExpr {
	if len(types) == 1 {
		if tuple, ok := types[0].(*ast.TupleTypeExpr); ok {
			return append([]ast.TypeExpr(nil), tuple.Elements...)
		}
	}
	return append([]ast.TypeExpr(nil), types...)
}

func returnExpressionIndex(stmts []ast.Stmt, fn *ast.FunctionExpr) (int, bool) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			for i, expr := range s.Exprs {
				if expr == ast.Expr(fn) {
					return i, true
				}
			}
		case *ast.IfStmt:
			if idx, ok := returnExpressionIndex(s.Then, fn); ok {
				return idx, true
			}
			if idx, ok := returnExpressionIndex(s.Else, fn); ok {
				return idx, true
			}
		case *ast.DoBlockStmt:
			if idx, ok := returnExpressionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.WhileStmt:
			if idx, ok := returnExpressionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.RepeatStmt:
			if idx, ok := returnExpressionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.NumberForStmt:
			if idx, ok := returnExpressionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.GenericForStmt:
			if idx, ok := returnExpressionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		}
	}
	return 0, false
}
