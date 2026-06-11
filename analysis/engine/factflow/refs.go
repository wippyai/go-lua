package factflow

// ExprRef is an opaque, comparable reference to a source expression.
type ExprRef uint32

// TypeRef is an opaque, comparable reference to a source type expression.
type TypeRef uint32

func copyTypeRefs(in []TypeRef) []TypeRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]TypeRef, len(in))
	copy(out, in)
	return out
}
