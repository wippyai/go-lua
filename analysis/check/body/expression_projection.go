package body

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ExpressionTypeBeforeBoundary projects expr to the best type known before the
// boundary effect at point. It is the canonical post-solve expression type
// fallback for diagnostics and obligations that need syntax-owned expression
// context without importing AST outside body.
func (r *Result) ExpressionTypeBeforeBoundary(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if r == nil || expr == nil {
		return nil, false
	}
	if value, ok := r.ExpressionValueBeforeBoundary(point, expr); ok {
		if t, typeOK := r.valueTypeWithPresence(value); typeOK && t != nil {
			return t, true
		}
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		if value, valueOK := r.CallExprResultValue(call, 0); valueOK {
			if t, typeOK := r.valueTypeWithPresence(value); typeOK && t != nil {
				return t, true
			}
		}
	}
	if p, pathOK := r.ExpressionPath(expr); pathOK && !p.IsEmpty() && p.Symbol != 0 {
		if declared, declaredOK := r.SymbolDeclaredType(p.Symbol); declaredOK {
			if len(p.Segments) == 0 {
				return declared, true
			}
			return luatypeprojection.ApplySegments(declared, p.Segments)
		}
	}
	return LiteralExpressionType(expr)
}

// LiteralExpressionType returns the static type of a literal expression.
func LiteralExpressionType(expr ast.Expr) (typ.Type, bool) {
	return valueexpr.LiteralType(expr)
}

// DeclaredExpressionTypeAt resolves expr through declared annotation types and
// dominating declaration sources visible at point.
func (r *Result) DeclaredExpressionTypeAt(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if t, ok := r.declaredExpressionType(expr); ok {
		return t, true
	}
	if r != nil && expr != nil {
		if p, ok := r.ExpressionPath(expr); ok && !p.IsEmpty() && p.Symbol != 0 {
			if t, ok := r.dominatingDeclarationSourcePathType(point, p); ok {
				return t, true
			}
		}
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := r.DeclaredExpressionTypeAt(point, attr.Object)
	if !ok || container == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		return access.Field(container, name)
	}
	key, ok := valueexpr.LiteralType(attr.Key)
	if !ok {
		key, ok = r.ExpressionTypeBeforeBoundary(point, attr.Key)
	}
	if !ok || key == nil {
		return nil, false
	}
	return access.RuntimeIndex(container, key)
}

func (r *Result) declaredExpressionType(expr ast.Expr) (typ.Type, bool) {
	if r == nil || expr == nil {
		return nil, false
	}
	p, ok := r.ExpressionPath(expr)
	if !ok || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	return r.declaredPathType(p)
}

func (r *Result) DeclaredPathTypeAt(point cfg.Point, p pathdom.Path, ok bool) (typ.Type, bool) {
	if !ok || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	if t, typeOK := r.DeclaredPathType(p); typeOK {
		return t, true
	}
	return r.dominatingDeclarationSourcePathType(point, p)
}

func (r *Result) DeclaredPathType(p pathdom.Path) (typ.Type, bool) {
	return r.declaredPathType(p)
}

func (r *Result) declaredPathType(p pathdom.Path) (typ.Type, bool) {
	if p.Symbol == 0 {
		return nil, false
	}
	declared, ok := r.SymbolDeclaredType(p.Symbol)
	if !ok || declared == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return declared, true
	}
	return luatypeprojection.ApplySegments(declared, p.Segments)
}

func (r *Result) dominatingDeclarationSourcePathType(point cfg.Point, p pathdom.Path) (typ.Type, bool) {
	if r == nil || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p)
	if !ok || !declaration.Source.HasExpr {
		return nil, false
	}
	sourcePath, ok := r.ExpressionRefPath(declaration.Source.ExprRef)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return nil, false
	}
	return r.declaredPathType(sourcePath.AppendSegments(p.Segments))
}

// SymbolDeclaredType resolves the annotation type declared for id.
func (r *Result) SymbolDeclaredType(id symbol.ID) (typ.Type, bool) {
	if r == nil || id == 0 {
		return nil, false
	}
	expr, ok := r.SymbolTypeAnnotation(id)
	if !ok || expr == nil || r.TypeResolver() == nil {
		return nil, false
	}
	return r.TypeResolver().Type(expr)
}

func (r *Result) SymbolHasTypeAnnotation(id symbol.ID) bool {
	if r == nil || id == 0 {
		return false
	}
	expr, ok := r.SymbolTypeAnnotation(id)
	return ok && expr != nil
}

func (r *Result) valueTypeWithPresence(value product.Value) (typ.Type, bool) {
	if r == nil || r.registry == nil || r.typeValues == nil {
		return nil, false
	}
	return proof.New(r.registry, r.typeValues).ValueTypeWithPresence(value)
}

// ExpressionLabel returns a compact user-facing expression label.
func ExpressionLabel(expr ast.Expr) string {
	return expressionLabelDepth(expr, 0)
}

func expressionLabelDepth(expr ast.Expr, depth int) string {
	if depth > typ.DefaultRecursionDepth {
		return ""
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := expressionLabelDepth(e.Object, depth+1)
		key := attrKeyLabel(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.FuncCallExpr:
		return callLabelDepth(e, depth+1)
	case *ast.CastExpr:
		return expressionLabelDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		return expressionLabelDepth(e.Expr, depth+1)
	default:
		return ""
	}
}

func callLabelDepth(expr *ast.FuncCallExpr, depth int) string {
	if depth > typ.DefaultRecursionDepth || expr == nil {
		return ""
	}
	if expr.Receiver != nil && expr.Method != "" {
		receiver := expressionLabelDepth(expr.Receiver, depth+1)
		if receiver == "" {
			return ""
		}
		return receiver + ":" + expr.Method + "(...)"
	}
	name := expressionLabelDepth(expr.Func, depth+1)
	if name == "" {
		return ""
	}
	return name + "(...)"
}

func attrKeyLabel(expr *ast.AttrGetExpr) string {
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

func expressionKey(point cfg.Point, expr ast.Expr) string {
	span := ast.SpanOf(expr)
	return strconv.Itoa(int(point)) + ":" +
		strconv.Itoa(span.StartLine) + ":" +
		strconv.Itoa(span.StartCol) + ":" +
		strconv.Itoa(span.EndLine) + ":" +
		strconv.Itoa(span.EndCol)
}
