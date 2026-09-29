package typ

// WriteInto returns the type of a table value of type t after a write into
// it: a field or index write, direct, through a callee, or summarized into an
// overlay. It is the one rule for writes into a dynamic value: a value typed
// any admits every write and stays any, so the fields and entries it holds
// stay dynamic. Any other type takes the type widen derives for the write.
func WriteInto(t Type, widen func(Type) Type) Type {
	if t != nil && IsAny(t) {
		return t
	}
	return widen(t)
}
