// Package pathexpr resolves Lua AST expressions into analysis access paths.
package pathexpr

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Resolve extracts a static access path from expr using lexical binding data.
//
// Supported forms are identifiers, dot fields, string indexes, integer indexes,
// and nested combinations of those forms. Dynamic indexes are rejected.
func Resolve(expr ast.Expr, bindings *bind.Result) (path.Path, bool) {
	switch expr := expr.(type) {
	case *ast.IdentExpr:
		return resolveIdent(expr, bindings)
	case *ast.AttrGetExpr:
		return resolveAttr(expr, bindings)
	default:
		return path.Path{}, false
	}
}

// ResolveContainer extracts the receiver/container path for an attribute/index
// expression. The full expression may still be unresolvable when its key is
// dynamic.
func ResolveContainer(expr ast.Expr, bindings *bind.Result) (path.Path, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		return path.Path{}, false
	}
	return Resolve(attr.Object, bindings)
}

func resolveIdent(expr *ast.IdentExpr, bindings *bind.Result) (path.Path, bool) {
	if expr == nil || bindings == nil {
		return path.Path{}, false
	}
	id, ok := bindings.SymbolOf(expr)
	if !ok || id == 0 {
		return path.Path{}, false
	}
	name := bindings.Name(id)
	if name == "" {
		name = expr.Value
	}
	return path.NewPath(id, name), true
}

func resolveAttr(expr *ast.AttrGetExpr, bindings *bind.Result) (path.Path, bool) {
	if expr == nil {
		return path.Path{}, false
	}
	base, ok := Resolve(expr.Object, bindings)
	if !ok {
		return path.Path{}, false
	}

	switch key := expr.Key.(type) {
	case *ast.StringExpr:
		switch expr.KeySyntax {
		case ast.AttrKeyDot:
			if key.Value == "" {
				return path.Path{}, false
			}
			return base.Field(key.Value), true
		case ast.AttrKeyIndex:
			return base.IndexStr(key.Value), true
		default:
			if isIdentName(key.Value) {
				return base.Field(key.Value), true
			}
			return base.IndexStr(key.Value), true
		}
	case *ast.NumberExpr:
		index, ok := parseNonNegativeDecimalInt(key.Value)
		if !ok {
			return path.Path{}, false
		}
		return base.IndexInt(index), true
	default:
		return path.Path{}, false
	}
}

func parseNonNegativeDecimalInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	value := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		digit := int(ch - '0')
		if value > (maxInt-digit)/10 {
			return 0, false
		}
		value = value*10 + digit
	}
	return value, true
}

func isIdentName(s string) bool {
	if s == "" {
		return false
	}
	if !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentContinue(s[i]) {
			return false
		}
	}
	return true
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentContinue(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}
