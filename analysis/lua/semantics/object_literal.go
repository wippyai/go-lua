package semantics

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractObjectLiterals(exprs []ast.Expr, resolver sourceprovenance.CallPointResolver) {
	if r == nil {
		return
	}
	for _, expr := range exprs {
		r.extractObjectLiteral(expr, resolver)
	}
}

func (r *Result) extractObjectLiteral(expr ast.Expr, resolver sourceprovenance.CallPointResolver) {
	table, ok := pathexpr.ObjectLiteralTable(expr)
	if ok && table != nil {
		if _, exists := r.objectLiterals[expr]; !exists {
			base := pathexpr.ObjectEntries(table)
			entries := make([]ObjectEntryFact, len(base))
			for i, entry := range base {
				entries[i] = ObjectEntryFact{
					Field:      entry.Field,
					Index:      entry.Index,
					Key:        entry.Key,
					Value:      entry.Value,
					ValueSpan:  sourceSpanOf(entry.Value),
					ValueLabel: expressionLabel(entry.Value),
					Suffix:     entry.Suffix,
					Source:     objectEntryValueSource(entry.Value, entry.Field != nil && entry.Field.Key == nil && entry.Final, resolver),
				}
			}
			r.objectLiterals[expr] = ObjectLiteralFact{
				Expr:    expr,
				Table:   table,
				Entries: entries,
			}
		}
		for _, field := range table.Fields {
			if field == nil {
				continue
			}
			r.extractObjectLiteral(field.Value, resolver)
		}
		return
	}

	switch expr := expr.(type) {
	case *ast.LogicalOpExpr:
		r.extractObjectLiteral(expr.Lhs, resolver)
		r.extractObjectLiteral(expr.Rhs, resolver)
	case *ast.RelationalOpExpr:
		r.extractObjectLiteral(expr.Lhs, resolver)
		r.extractObjectLiteral(expr.Rhs, resolver)
	case *ast.StringConcatOpExpr:
		r.extractObjectLiteral(expr.Lhs, resolver)
		r.extractObjectLiteral(expr.Rhs, resolver)
	case *ast.ArithmeticOpExpr:
		r.extractObjectLiteral(expr.Lhs, resolver)
		r.extractObjectLiteral(expr.Rhs, resolver)
	case *ast.UnaryMinusOpExpr:
		r.extractObjectLiteral(expr.Expr, resolver)
	case *ast.UnaryNotOpExpr:
		r.extractObjectLiteral(expr.Expr, resolver)
	case *ast.UnaryLenOpExpr:
		r.extractObjectLiteral(expr.Expr, resolver)
	case *ast.UnaryBNotOpExpr:
		r.extractObjectLiteral(expr.Expr, resolver)
	case *ast.AttrGetExpr:
		r.extractObjectLiteral(expr.Object, resolver)
		r.extractObjectLiteral(expr.Key, resolver)
	case *ast.CastExpr:
		if _, ok := sourceprovenance.ProofInner(expr); !ok {
			return
		}
		r.extractObjectLiteral(expr.Expr, resolver)
	case *ast.NonNilAssertExpr:
		r.extractObjectLiteral(expr.Expr, resolver)
	case *ast.FuncCallExpr:
		r.extractObjectLiterals(expr.Args, resolver)
	}
}

func objectEntryValueSource(expr ast.Expr, final bool, resolver sourceprovenance.CallPointResolver) sourceprovenance.ASTSource {
	return sourceprovenance.SourceForExpr(expr, sourceprovenance.NoSourceIndex, sourceprovenance.NoSourceIndex, 0, final, false, resolver)
}

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
