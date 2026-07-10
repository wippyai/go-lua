package sourceprovenance

import (
	"reflect"

	"github.com/wippyai/go-lua/compiler/ast"
)

func exprNil(expr ast.Expr) bool {
	if expr == nil {
		return true
	}
	value := reflect.ValueOf(expr)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
