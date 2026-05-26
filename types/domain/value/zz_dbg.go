package value

import "github.com/wippyai/go-lua/types/typ"

// Diagnostic helpers for the recursive-family convergence work. Retained until the
// abstract interpreter fully converges; removed only in the final de-scatter pass.

// DbgString renders a type for convergence diagnostics.
func DbgString(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.String()
}

// ContainsRecursiveDbg reports whether t carries recursive structure.
func ContainsRecursiveDbg(t typ.Type) bool {
	return typ.ContainsRecursive(t)
}

// FamilyHashDbg is the metadata-blind coinductive product-family fingerprint.
func FamilyHashDbg(t typ.Type) uint64 {
	return typ.ProductFamilyHash(t)
}

// CanonicalSameDbg reports whether two types canonicalize to the same recursive
// family representative.
func CanonicalSameDbg(a, b typ.Type) bool {
	return CanonicalRecursiveFamily(a) == CanonicalRecursiveFamily(b)
}

// MetatableDbg renders the recursive metatable carried by a record value, the
// part String() elides, so convergence diagnostics expose the family that
// differs between two observations.
func MetatableDbg(t typ.Type) string {
	rec, ok := UnwrapStructuralShape(t).(*typ.Record)
	if !ok || rec == nil || rec.Metatable == nil {
		return "<no-mt>"
	}
	return rec.Metatable.String()
}
