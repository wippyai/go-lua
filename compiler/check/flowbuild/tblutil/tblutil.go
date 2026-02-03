package tblutil

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

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
		return typ.NewRecord().Build()
	}

	builder := typ.NewRecord()
	var arrayElements []typ.Type
	hasVararg := false
	fieldCount := 0

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
		switch k := field.Key.(type) {
		case *ast.StringExpr:
			builder.Field(k.Value, recurse(field.Value, p))
			fieldCount++
		case *ast.IdentExpr:
			builder.Field(k.Value, recurse(field.Value, p))
			fieldCount++
		}
	}

	if len(arrayElements) > 0 && fieldCount == 0 {
		if hasVararg {
			return typ.NewArray(typ.NewUnion(arrayElements...))
		}
		return typ.NewTuple(arrayElements...)
	}

	return builder.Build()
}
