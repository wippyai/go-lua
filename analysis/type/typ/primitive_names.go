package typ

// BuiltinPrimitiveName reports whether name is one of the built-in primitive
// type names.
func BuiltinPrimitiveName(name string) bool {
	_, ok := BuiltinPrimitiveType(name)
	return ok
}

// BuiltinPrimitiveType returns the singleton type for a built-in primitive
// name.
func BuiltinPrimitiveType(name string) (Type, bool) {
	switch name {
	case "nil":
		return Nil, true
	case "boolean":
		return Boolean, true
	case "number":
		return Number, true
	case "integer":
		return Integer, true
	case "string":
		return String, true
	case "any":
		return Any, true
	case "unknown":
		return Unknown, true
	case "never":
		return Never, true
	case "self":
		return Self, true
	default:
		return nil, false
	}
}
