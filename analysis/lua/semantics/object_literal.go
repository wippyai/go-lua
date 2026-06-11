package semantics

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
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
	if field == nil {
		return path.Path{}, false
	}
	if field.Key == nil {
		*arrayIndex = *arrayIndex + 1
		if finalField {
			expanded, _, _ := sourceprovenance.ValueShape(field.Value, true, true, false)
			if expanded {
				return path.Path{}, false
			}
		}
		return suffix(segment.Segment{Kind: segment.SegmentIndexInt, Index: *arrayIndex}), true
	}
	switch key := field.Key.(type) {
	case *ast.StringExpr:
		switch field.KeySyntax {
		case ast.AttrKeyDot:
			if key.Value == "" {
				return path.Path{}, false
			}
			return suffix(segment.Segment{Kind: segment.SegmentField, Name: key.Value}), true
		case ast.AttrKeyIndex:
			return suffix(segment.Segment{Kind: segment.SegmentIndexString, Name: key.Value}), true
		default:
			return path.Path{}, false
		}
	case *ast.NumberExpr:
		if field.KeySyntax != ast.AttrKeyIndex {
			return path.Path{}, false
		}
		index, ok := parseStaticNonNegativeDecimalInt(key.Value)
		if !ok {
			return path.Path{}, false
		}
		return suffix(segment.Segment{Kind: segment.SegmentIndexInt, Index: index}), true
	default:
		return path.Path{}, false
	}
}

func lastTableFieldIndex(fields []*ast.Field) int {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i] != nil {
			return i
		}
	}
	return -1
}

func suffix(seg segment.Segment) path.Path {
	return path.Path{Segments: []segment.Segment{seg}}
}

func appendSuffix(prefix path.Path, suffix path.Path) path.Path {
	out := copyPath(prefix)
	out.Segments = append(out.Segments, suffix.Segments...)
	return out
}

func objectEntryValueSource(expr ast.Expr, final bool) sourceprovenance.ASTSource {
	return sourceprovenance.SourceForExpr(expr, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, final, false, nil)
}

func parseStaticNonNegativeDecimalInt(s string) (int, bool) {
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
