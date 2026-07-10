package typ

const BuiltinTableTopName = "table"

// BuiltinTableTopMarker returns the canonical source-level Lua `table` type.
// It is represented as an empty nominal interface so the low-level type
// vocabulary can recognize it without importing table-specific helper packages.
func BuiltinTableTopMarker() Type {
	return NewInterface(BuiltinTableTopName, nil)
}

// IsBuiltinTableTopMarker reports whether t is the builtin Lua `table` top
// marker. The marker means "some table-like shape", not a closed interface.
func IsBuiltinTableTopMarker(t Type) bool {
	iface, ok := t.(*Interface)
	return ok && iface.Name == BuiltinTableTopName && len(iface.Methods) == 0
}
