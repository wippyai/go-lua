package tblutil

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// MapLiteralKeyCardinality returns the proven key-count lower bound of a pure
// keyed-map constructor: the number of distinct static keys whose value is not a
// nil literal. A pure map ({["a"]=1,["b"]=2}) has an empty sequence part (#t==0),
// so its cardinality is the entry count, the relation a keys-collecting function
// ties its returned array length to.
//
// The count is a sound lower bound on that key cardinality: duplicate static keys
// collapse to one entry, a dynamic key proves no new distinct entry (it may alias
// an existing key), and a nil-literal value writes no entry. A constructor with
// any positional field is not a pure map and yields 0, leaving the positional
// sequence-length channel to own its length.
func MapLiteralKeyCardinality(tbl *ast.TableExpr) int64 {
	if tbl == nil || len(tbl.Fields) == 0 {
		return 0
	}
	seen := make(map[constraint.Segment]struct{}, len(tbl.Fields))
	for _, field := range tbl.Fields {
		if field == nil || field.Value == nil {
			return 0
		}
		if field.Key == nil {
			return 0
		}
		if _, ok := field.Value.(*ast.NilExpr); ok {
			continue
		}
		seg, ok := pathseg.StaticTableFieldSegment(field)
		if !ok {
			continue
		}
		seen[seg] = struct{}{}
	}
	return int64(len(seen))
}

// TableHasFunctionField reports whether the table literal has any function-valued fields.
func TableHasFunctionField(ex *ast.TableExpr) bool {
	if ex == nil {
		return false
	}
	for _, field := range ex.Fields {
		if field == nil || field.Value == nil {
			continue
		}
		if _, ok := field.Value.(*ast.FunctionExpr); ok {
			return true
		}
	}
	return false
}

// SynthTableLiteralWithWrapper synthesizes a table literal using the provided wrapper for field values.
// This is used to propagate known loop var types into table literal fields during extraction.
func SynthTableLiteralWithWrapper(ex *ast.TableExpr, p cfg.Point, recurse func(ast.Expr, cfg.Point) typ.Type) typ.Type {
	if ex == nil {
		return nil
	}
	if len(ex.Fields) == 0 {
		return typ.NewRecord().SetOpen(true).Build()
	}

	builder := typ.NewRecord()
	var arrayElements []typ.Type
	var indexedWrites []struct {
		key typ.Type
		val typ.Type
	}
	hasVararg := false
	keyedCount := 0

	for _, field := range ex.Fields {
		if field == nil {
			continue
		}
		if field.Key == nil {
			if _, ok := field.Value.(*ast.Comma3Expr); ok {
				hasVararg = true
			}
			arrayElements = append(arrayElements, recurse(field.Value, p))
			continue
		}
		seg, ok := pathseg.StaticTableFieldSegment(field)
		if !ok {
			continue
		}
		val := recurse(field.Value, p)
		switch seg.Kind {
		case constraint.SegmentField:
			builder.Field(seg.Name, val)
			keyedCount++
		case constraint.SegmentIndexString:
			indexedWrites = append(indexedWrites, struct {
				key typ.Type
				val typ.Type
			}{key: typ.LiteralString(seg.Name), val: val})
			keyedCount++
		case constraint.SegmentIndexInt:
			indexedWrites = append(indexedWrites, struct {
				key typ.Type
				val typ.Type
			}{key: typ.LiteralInt(int64(seg.Index)), val: val})
			keyedCount++
		}
	}

	if len(arrayElements) > 0 && keyedCount == 0 {
		if hasVararg {
			return typ.NewArray(typ.NewUnion(arrayElements...))
		}
		return typ.NewTuple(arrayElements...)
	}

	var out typ.Type = builder.Build()
	for _, write := range indexedWrites {
		out = value.AdmitForeignIndexedWrite(out, write.key, write.val)
	}
	return out
}

// FunctionHasAnnotations returns true if the function expression has explicit type annotations
// (parameter types or return types).
func FunctionHasAnnotations(fn *ast.FunctionExpr) bool {
	if fn == nil {
		return false
	}
	for _, rt := range fn.ReturnTypes {
		if rt != nil {
			return true
		}
	}
	if fn.ParList != nil {
		for _, pt := range fn.ParList.Types {
			if pt != nil {
				return true
			}
		}
		if fn.ParList.VarargType != nil {
			return true
		}
	}
	return false
}
