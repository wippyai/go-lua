package typeresolve

// BuiltinPrimitiveName reports whether name is a built-in primitive type name.
func BuiltinPrimitiveName(name string) bool {
	switch name {
	case "nil", "boolean", "number", "integer", "string", "any", "unknown", "never", "self":
		return true
	default:
		return false
	}
}
