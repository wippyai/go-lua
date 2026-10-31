package core

import (
	"github.com/wippyai/go-lua/compiler/ast"
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
