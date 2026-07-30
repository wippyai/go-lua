package expressionid

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Of returns the opaque process-local expression identity used to join WIR
// source metadata to factflow expression facts during the migration away from
// AST sidecars. The returned value must not be serialized.
func Of(expr ast.Expr) wir.ExpressionID {
	if expr == nil {
		return 0
	}
	v := reflect.ValueOf(expr)
	if v.Kind() != reflect.Pointer && v.Kind() != reflect.UnsafePointer {
		return 0
	}
	return wir.ExpressionID(v.Pointer())
}
