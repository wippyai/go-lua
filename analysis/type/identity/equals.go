package identity

import "github.com/wippyai/go-lua/analysis/type/typ"

// SameNode reports whether two Type interface values point at the same
// immutable type node.
func SameNode(a, b typ.Type) bool {
	return typ.SameNode(a, b)
}

// TypeEquals compares two types for structural equality with cycle detection.
func TypeEquals(a, b typ.Type) bool {
	return typ.TypeEquals(a, b)
}

// SameNodeOrAcyclicEqual reports identity or structural equality for products
// that cannot contain recursive cycles.
func SameNodeOrAcyclicEqual(a, b typ.Type) bool {
	return typ.SameNodeOrAcyclicEqual(a, b)
}

// NormalizeNilType converts typed nil Type implementations to nil.
func NormalizeNilType(t typ.Type) typ.Type {
	return typ.NormalizeNilType(t)
}
