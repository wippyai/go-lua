package semantics

import (
	"strconv"

	"github.com/wippyai/go-lua/compiler/ast"
)

func sourceSpanOf(expr ast.Expr) SourceSpan {
	if expr == nil {
		return SourceSpan{}
	}
	span := ast.SpanOf(expr)
	if ident, ok := expr.(*ast.IdentExpr); ok && span.Valid() && span.EndLine == span.StartLine && span.EndCol <= span.StartCol && ident.Value != "" {
		span.EndCol = span.StartCol + len(ident.Value)
	}
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func expressionLabel(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := expressionLabel(e.Object)
		key := attrKeyLabel(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.CastExpr:
		return expressionLabel(e.Expr)
	case *ast.NonNilAssertExpr:
		return expressionLabel(e.Expr)
	case *ast.FuncCallExpr:
		if unpackCallLabel(e) {
			return "unpack(...)"
		}
		return ""
	default:
		return ""
	}
}

func attrKeyLabel(expr *ast.AttrGetExpr) string {
	if expr == nil {
		return ""
	}
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

func unpackCallLabel(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" || call.Receiver != nil {
		return false
	}
	if ident, ok := call.Func.(*ast.IdentExpr); ok {
		return ident.Value == "unpack"
	}
	attr, ok := call.Func.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	obj, ok := attr.Object.(*ast.IdentExpr)
	if !ok || obj.Value != "table" {
		return false
	}
	key, ok := attr.Key.(*ast.StringExpr)
	return ok && key.Value == "unpack"
}
