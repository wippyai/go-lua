package semantics

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractObjectLiterals(exprs []ast.Expr) {
	if r == nil {
		return
	}
	for _, expr := range exprs {
		r.extractObjectLiteral(expr)
	}
}

func (r *Result) extractObjectLiteral(expr ast.Expr) {
	table, ok := objectLiteralTable(expr)
	if !ok || table == nil {
		return
	}
	if _, exists := r.objectLiterals[expr]; !exists {
		if entries := objectEntries(table); len(entries) != 0 {
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
		r.extractObjectLiteral(field.Value)
	}
}

func objectLiteralTable(expr ast.Expr) (*ast.TableExpr, bool) {
	for {
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			expr = wrapped.Expr
		case *ast.TableExpr:
			return wrapped, true
		default:
			return nil, false
		}
	}
}

func objectEntries(table *ast.TableExpr) []ObjectEntryFact {
	if table == nil || len(table.Fields) == 0 {
		return nil
	}
	entries := make([]ObjectEntryFact, 0, len(table.Fields))
	arrayIndex := 0
	finalField := lastTableFieldIndex(table.Fields)
	for i, field := range table.Fields {
		if field == nil {
			continue
		}
		suffix, ok := objectEntrySuffix(field, &arrayIndex, i == finalField)
		if !ok {
			continue
		}
		entries = append(entries, ObjectEntryFact{
			Field:  field,
			Index:  i,
			Key:    field.Key,
			Value:  field.Value,
			Suffix: suffix,
			Source: objectEntryValueSource(field.Value, field.Key == nil && i == finalField),
		})
		if nested, ok := objectLiteralTable(field.Value); ok {
			entries = append(entries, nestedObjectEntries(nested, suffix)...)
		}
	}
	return entries
}

func nestedObjectEntries(table *ast.TableExpr, prefix path.Path) []ObjectEntryFact {
	entries := objectEntries(table)
	if len(entries) == 0 {
		return nil
	}
	for i := range entries {
		entries[i].Suffix = appendSuffix(prefix, entries[i].Suffix)
	}
	return entries
}

func objectEntrySuffix(field *ast.Field, arrayIndex *int, finalField bool) (path.Path, bool) {
	suffix, ok := pathexpr.ResolveTableFieldSuffix(field, arrayIndex)
	if !ok {
		return path.Path{}, false
	}
	if field.Key == nil {
		if finalField {
			expanded, _, _ := sourceprovenance.ValueShape(field.Value, true, true, false)
			if expanded {
				return path.Path{}, false
			}
		}
	}
	return suffix.Path, true
}

func lastTableFieldIndex(fields []*ast.Field) int {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i] != nil {
			return i
		}
	}
	return -1
}

func appendSuffix(prefix path.Path, suffix path.Path) path.Path {
	out := copyPath(prefix)
	out.Segments = append(out.Segments, suffix.Segments...)
	return out
}

func objectEntryValueSource(expr ast.Expr, final bool) sourceprovenance.ASTSource {
	return sourceprovenance.SourceForExpr(expr, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, final, false, nil)
}
