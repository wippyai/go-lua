package exprdisplay

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Name returns a readable, bounded source expression name or fallback.
func Name(expr ast.Expr, fallback string) string {
	if name := NameOK(expr); name != "" {
		return name
	}
	return fallback
}

// NameOK returns a readable, bounded source expression name when the expression
// is simple enough to display without implying a semantic proof.
func NameOK(expr ast.Expr) string {
	return nameOKDepth(expr, 0)
}

func nameOKDepth(expr ast.Expr, depth int) string {
	if depth > typ.DefaultRecursionDepth {
		return ""
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := nameOKDepth(e.Object, depth+1)
		key := attrKeyName(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.FuncCallExpr:
		return callNameOKDepth(e, depth+1)
	case *ast.CastExpr:
		return nameOKDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		return nameOKDepth(e.Expr, depth+1)
	default:
		return ""
	}
}

func callNameOKDepth(expr *ast.FuncCallExpr, depth int) string {
	if depth > typ.DefaultRecursionDepth || expr == nil {
		return ""
	}
	if expr.Receiver != nil && expr.Method != "" {
		receiver := nameOKDepth(expr.Receiver, depth+1)
		if receiver == "" {
			return ""
		}
		return receiver + ":" + expr.Method + "(...)"
	}
	name := nameOKDepth(expr.Func, depth+1)
	if name == "" {
		return ""
	}
	return name + "(...)"
}

func attrKeyName(expr *ast.AttrGetExpr) string {
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		if name := ast.KeyName(expr.Key); name != "" {
			return "." + name
		}
	case ast.AttrKeyIndex:
		switch key := expr.Key.(type) {
		case *ast.StringExpr:
			return "[" + strconv.Quote(key.Value) + "]"
		case *ast.NumberExpr:
			return "[" + key.Value + "]"
		case *ast.IdentExpr:
			return "[" + key.Value + "]"
		}
	}
	if name := ast.KeyName(expr.Key); name != "" {
		return "." + name
	}
	return ""
}
