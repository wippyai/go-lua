package castsem

// IsAnyTarget reports whether a primitive cast target is the explicit top-like
// `any` cast. It is a claim about precision, not runtime validation.
func IsAnyTarget(name string) bool {
	return name == "any"
}

// IsUnknownTarget reports whether a primitive cast target is explicitly
// unknown/top-like for diagnostic purposes.
func IsUnknownTarget(name string) bool {
	return name == "unknown"
}

// IsTopLikeTarget reports whether a primitive cast target is a precision
// boundary rather than concrete runtime validation.
func IsTopLikeTarget(name string) bool {
	return IsAnyTarget(name) || IsUnknownTarget(name)
}
