package typeresolve

import "github.com/wippyai/go-lua/analysis/type/typ"

// BuiltinPrimitiveName reports whether name is a built-in primitive type name.
func BuiltinPrimitiveName(name string) bool {
	return typ.BuiltinPrimitiveName(name)
}
