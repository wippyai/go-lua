// Package valueexpr owns syntactic Lua expression value classification shared
// by transfer facts and diagnostics, including assertion/cast unwrapping.
package valueexpr

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

// LiteralType returns the canonical literal type for syntactically obvious
// runtime literals. It looks through assertion wrappers that do not change the
// underlying runtime value.
func LiteralType(expr ast.Expr) (typ.Type, bool) {
	switch inner := sourceprovenance.AssertionInner(expr).(type) {
	case *ast.NilExpr:
		return typ.Nil, true
	case *ast.TrueExpr:
		return typ.LiteralBool(true), true
	case *ast.FalseExpr:
		return typ.LiteralBool(false), true
	case *ast.StringExpr:
		return typ.LiteralString(inner.Value), true
	case *ast.NumberExpr:
		if i, ok := numparse.ParseIntegerLiteral(inner.Value); ok {
			return typ.LiteralInt(i), true
		}
		if f, ok := numparse.ParseFloatLiteral(inner.Value); ok {
			return typ.LiteralNumber(f), true
		}
		return nil, false
	default:
		return nil, false
	}
}

// RuntimeKind returns the syntactically obvious runtime type tag for an
// expression. It only unwraps assertion/cast wrappers that do not alter the
// underlying runtime value.
func RuntimeKind(expr ast.Expr) (runtimekind.Value, bool) {
	switch sourceprovenance.AssertionInner(expr).(type) {
	case *ast.NilExpr:
		return runtimekind.Singleton(runtimekind.Nil), true
	case *ast.TrueExpr, *ast.FalseExpr:
		return runtimekind.Singleton(runtimekind.Boolean), true
	case *ast.NumberExpr:
		return runtimekind.Singleton(runtimekind.Number), true
	case *ast.StringExpr:
		return runtimekind.Singleton(runtimekind.String), true
	case *ast.TableExpr:
		return runtimekind.Singleton(runtimekind.Table), true
	case *ast.FunctionExpr:
		return runtimekind.Singleton(runtimekind.Function), true
	default:
		return runtimekind.Value{}, false
	}
}
