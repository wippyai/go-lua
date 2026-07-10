package pathexpr

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ObjectEntry describes one statically recoverable table-constructor entry and
// the suffix it contributes to the object's path identity.
type ObjectEntry struct {
	Field  *ast.Field
	Index  int
	Key    ast.Expr
	Value  ast.Expr
	Suffix path.Path
	Final  bool
}

// ObjectEntries resolves the static suffixes contributed by a table constructor
// and flattens nested object literals using the same path-prefix assembly that
// semantics previously owned.
func ObjectEntries(table *ast.TableExpr) []ObjectEntry {
	if table == nil || len(table.Fields) == 0 {
		return nil
	}
	return objectEntries(table, path.Path{})
}

// ObjectLiteralTable unwraps assertion/cast wrappers and returns the underlying
// table constructor when present.
func ObjectLiteralTable(expr ast.Expr) (*ast.TableExpr, bool) {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return nil, false
	}
	table, ok := inner.(*ast.TableExpr)
	return table, ok
}

func objectEntries(table *ast.TableExpr, prefix path.Path) []ObjectEntry {
	entries := make([]ObjectEntry, 0, len(table.Fields))
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
		suffix = prefix.AppendPathSuffix(suffix)
		entries = append(entries, ObjectEntry{
			Field:  field,
			Index:  i,
			Key:    field.Key,
			Value:  field.Value,
			Suffix: suffix,
			Final:  i == finalField,
		})
		if nested, ok := ObjectLiteralTable(field.Value); ok {
			entries = append(entries, objectEntries(nested, suffix)...)
		}
	}
	return entries
}

func objectEntrySuffix(field *ast.Field, arrayIndex *int, finalField bool) (path.Path, bool) {
	suffix, ok := ResolveTableFieldSuffix(field, arrayIndex)
	if !ok {
		return path.Path{}, false
	}
	if field.Key == nil && finalField {
		expanded, _, _ := sourceprovenance.ValueShape(field.Value, true, true, false)
		if expanded {
			return path.Path{}, false
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
