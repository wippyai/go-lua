package core

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// SharedTableValue reports whether a table value can have an existing alias.
func SharedTableValue(expr ast.Expr, t typ.Type) bool {
	_, fresh := expr.(*ast.TableExpr)
	if fresh {
		return false
	}
	switch value := unwrap.Alias(t).(type) {
	case *typ.Record, *typ.Array, *typ.Map, *typ.Tuple, *typ.Recursive:
		return true
	case *typ.Optional:
		return SharedTableValue(expr, value.Inner)
	case *typ.Union:
		for _, member := range value.Members {
			if SharedTableValue(expr, member) {
				return true
			}
		}
	}
	return false
}

// SharedTableAlternatives retains the mutable domains of logical value
// alternatives before joining their read types can erase empty-table evidence.
func SharedTableAlternatives(expr ast.Expr, synth func(ast.Expr) typ.Type) []typ.Type {
	logical, ok := expr.(*ast.LogicalOpExpr)
	if !ok {
		value := synth(expr)
		if SharedTableValue(expr, value) {
			return []typ.Type{value}
		}
		return nil
	}
	var values []typ.Type
	if logical.Operator == "or" {
		for _, value := range SharedTableAlternatives(logical.Lhs, synth) {
			values = append(values, narrow.ToTruthy(value))
		}
	}
	return append(values, SharedTableAlternatives(logical.Rhs, synth)...)
}
