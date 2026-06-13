package semantics

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
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
	if !ok || table == nil {
		return
	}
	if _, exists := r.objectLiterals[expr]; !exists {
		if base := pathexpr.ObjectEntries(table); len(base) != 0 {
			entries := make([]ObjectEntryFact, len(base))
			for i, entry := range base {
				entries[i] = ObjectEntryFact{
					Field:  entry.Field,
					Index:  entry.Index,
					Key:    entry.Key,
					Value:  entry.Value,
					Suffix: entry.Suffix,
					Source: objectEntryValueSource(entry.Value, entry.Field != nil && entry.Field.Key == nil && entry.Final, resolver),
				}
			}
			r.objectLiterals[expr] = ObjectLiteralFact{
				Expr:    expr,
				Table:   table,
				Entries: entries,
			}
		}
	}
	for _, field := range table.Fields {
		if field == nil {
			continue
		}
		r.extractObjectLiteral(field.Value, resolver)
	}
}

func objectEntryValueSource(expr ast.Expr, final bool, resolver sourceprovenance.CallPointResolver) sourceprovenance.ASTSource {
	return sourceprovenance.SourceForExpr(expr, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, final, false, resolver)
}
