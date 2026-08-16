package body

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

// expectedFunctionSignature resolves the contextual function type a function
// literal is checked against, when the literal sits in a return position whose
// enclosing function declares a function-typed return. Un-annotated parameters
// of the literal then adopt the declared parameter type at their position rather
// than a gradual-top seed, matching the declared contract instead of widening to
// any.
func expectedFunctionSignature(bindings *bind.Result, resolver *typeresolve.Resolver, fn *ast.FunctionExpr) (*typ.Function, bool) {
	parent, ok := bindings.ParentFunction(fn)
	if !ok || parent == nil {
		return nil, false
	}
	idx, ok := returnPositionIndex(parent.Stmts, fn)
	if !ok {
		return nil, false
	}
	declared := declaredReturnTypeExprs(parent.ReturnTypes)
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

// returnPositionIndex reports the return-expression index a function literal
// occupies within the first return statement that names it directly, scanning
// nested control-flow blocks. The index pairs the literal with the declared
// return type at the same position.
func returnPositionIndex(stmts []ast.Stmt, fn *ast.FunctionExpr) (int, bool) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			for i, expr := range s.Exprs {
				if expr == ast.Expr(fn) {
					return i, true
				}
			}
		case *ast.IfStmt:
			if idx, ok := returnPositionIndex(s.Then, fn); ok {
				return idx, true
			}
			if idx, ok := returnPositionIndex(s.Else, fn); ok {
				return idx, true
			}
		case *ast.DoBlockStmt:
			if idx, ok := returnPositionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.WhileStmt:
			if idx, ok := returnPositionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.RepeatStmt:
			if idx, ok := returnPositionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.NumberForStmt:
			if idx, ok := returnPositionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		case *ast.GenericForStmt:
			if idx, ok := returnPositionIndex(s.Stmts, fn); ok {
				return idx, true
			}
		}
	}
	return 0, false
}

// contextualParamType returns the declared parameter type at index from an
// expected function signature, skipping variadic and absent positions.
func contextualParamType(sig *typ.Function, index int) (typ.Type, bool) {
	if sig == nil || index < 0 || index >= len(sig.Params) {
		return nil, false
	}
	t := sig.Params[index].Type
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return nil, false
	}
	return t, true
}

// methodReceiverType resolves the declared type of a colon-method's implicit
// self receiver from the sibling type that shares the receiver's name. It
// returns the type only when it resolves to a concrete record-like type; an
// any/unknown receiver keeps the gradual-top seed so projection stays sound.
func methodReceiverType(bindings *bind.Result, resolver *typeresolve.Resolver, fn *ast.FunctionExpr) (typ.Type, bool) {
	decl, ok := bindings.MethodReceiverType(fn)
	if !ok {
		return nil, false
	}
	t, ok := resolver.Decl(decl)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return nil, false
	}
	return t, true
}
