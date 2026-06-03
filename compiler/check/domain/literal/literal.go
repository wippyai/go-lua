package literal

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// IsNilExpr checks if an expression is a nil literal.
func IsNilExpr(expr ast.Expr) bool {
	_, ok := expr.(*ast.NilExpr)
	return ok
}

// FromExpr extracts a literal type from an expression.
func FromExpr(expr ast.Expr) (*typ.Literal, bool) {
	switch v := expr.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(v.Value), true
	case *ast.NumberExpr:
		if i, ok := numparse.ParseIntegerLiteral(v.Value); ok {
			return typ.LiteralInt(i), true
		}
		if f, ok := numparse.ParseFloatLiteral(v.Value); ok {
			return typ.LiteralNumber(f), true
		}
	case *ast.TrueExpr:
		return typ.True, true
	case *ast.FalseExpr:
		return typ.False, true
	}
	return nil, false
}

// FromExprWithConst extracts a literal type from an expression,
// resolving identifiers via the const resolver if available.
func FromExprWithConst(expr ast.Expr, constResolver func(string) *flow.ConstValue) (*typ.Literal, bool) {
	if lit, ok := FromExpr(expr); ok {
		return lit, true
	}
	if ident, ok := expr.(*ast.IdentExpr); ok && constResolver != nil {
		if cv := constResolver(ident.Value); cv != nil {
			switch cv.Kind {
			case flow.ConstString:
				return typ.LiteralString(cv.Str), true
			case flow.ConstBool:
				if cv.Bool {
					return typ.True, true
				}
				return typ.False, true
			case flow.ConstInt:
				return typ.LiteralInt(cv.Int), true
			case flow.ConstFloat:
				return typ.LiteralNumber(cv.Float), true
			}
		}
	}
	return nil, false
}

// FromExprWithSymType extracts a literal type from an expression,
// first trying const resolution, then falling back to symbol type lookup.
// This handles module-level constants that are captured by inner functions.
func FromExprWithSymType(expr ast.Expr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool), p cfg.Point) (*typ.Literal, bool) {
	if lit, ok := FromExprWithConst(expr, constResolver); ok {
		return lit, true
	}
	if ident, ok := expr.(*ast.IdentExpr); ok && bindings != nil && symResolver != nil {
		sym, found := bindings.SymbolOf(ident)
		if found && sym != 0 {
			if t, ok := symResolver(p, sym); ok && t != nil {
				if lit, ok := t.(*typ.Literal); ok {
					return lit, true
				}
			}
		}
	}
	return nil, false
}

// KeyTypeFromExpr derives the most precise singleton key type from a literal
// expression. Returns nil if the expression is not a recognized literal kind.
func KeyTypeFromExpr(expr ast.Expr, constResolver func(string) *flow.ConstValue) typ.Type {
	if expr == nil {
		return nil
	}
	lit, ok := FromExprWithConst(expr, constResolver)
	if !ok || lit == nil {
		return nil
	}
	switch lit.Base {
	case kind.String:
		if v, ok := lit.Value.(string); ok {
			return typ.LiteralString(v)
		}
	case kind.Integer:
		if v, ok := lit.Value.(int64); ok {
			return typ.LiteralInt(v)
		}
	case kind.Number:
		if v, ok := lit.Value.(float64); ok {
			return typ.LiteralNumber(v)
		}
	case kind.Boolean:
		if v, ok := lit.Value.(bool); ok {
			return typ.LiteralBool(v)
		}
	}
	return nil
}
