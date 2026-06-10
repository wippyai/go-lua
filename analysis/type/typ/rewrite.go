package typ

// Rewrite traverses a type tree and applies fn at each node (bottom-up transformation).
//
// The function fn is called on each type node before recursing into children.
// If fn returns (replacement, true), the replacement is used and children are
// not visited (early termination). If fn returns (_, false), children are
// recursively rewritten first, then the result is reassembled.
//
// Returns the original pointer when nothing changed (structural sharing).
// This is the foundation for type substitution, expansion, and other transforms.
func Rewrite(t Type, fn func(Type) (Type, bool)) Type {
	return rewriteWithDepth(t, fn, DefaultRecursionDepth)
}
