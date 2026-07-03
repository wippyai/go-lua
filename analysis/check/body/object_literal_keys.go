package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ObjectLiteralStaticStringKeysAtPath returns the known string keys of an object
// literal or of a nested object literal at suffix. Array/int entries are ignored
// because they do not contribute to string-key dispatch domains; dynamic or
// invalid field keys make the key set unknown.
func (r *Result) ObjectLiteralStaticStringKeysAtPath(fact semantics.ObjectLiteralFact, suffix []segment.Segment) (map[string]bool, ast.Span, bool) {
	if len(suffix) != 0 {
		nested, ok := r.nestedObjectLiteralFact(fact, suffix)
		if !ok {
			return nil, ast.Span{}, false
		}
		fact = nested
	}
	keys, ok := staticStringKeysOfObjectLiteral(fact)
	if !ok {
		return nil, ast.Span{}, false
	}
	return keys, ast.SpanOf(fact.Table), true
}

func (r *Result) nestedObjectLiteralFact(fact semantics.ObjectLiteralFact, suffix []segment.Segment) (semantics.ObjectLiteralFact, bool) {
	if r == nil || len(suffix) == 0 {
		return semantics.ObjectLiteralFact{}, false
	}
	for _, entry := range fact.Entries {
		if !sameSegments(entry.Suffix.Segments, suffix) {
			continue
		}
		nested, ok := r.ObjectLiteral(entry.Value)
		return nested, ok
	}
	return semantics.ObjectLiteralFact{}, false
}

func staticStringKeysOfObjectLiteral(fact semantics.ObjectLiteralFact) (map[string]bool, bool) {
	if fact.Table == nil {
		return nil, false
	}
	keys := make(map[string]bool, len(fact.Table.Fields))
	arrayIndex := 0
	for _, field := range fact.Table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok {
			return nil, false
		}
		switch suffix.Kind {
		case pathexpr.TableFieldSuffixField, pathexpr.TableFieldSuffixStringIndex:
			if suffix.Segment.Name == "" {
				return nil, false
			}
			keys[suffix.Segment.Name] = true
		case pathexpr.TableFieldSuffixImplicitIndex, pathexpr.TableFieldSuffixIntIndex:
			continue
		default:
			return nil, false
		}
	}
	return keys, true
}

func sameSegments(a, b []segment.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
