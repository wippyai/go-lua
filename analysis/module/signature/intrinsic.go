package signature

// Intrinsic is a sealed semantic identity for a canonical native operation
// whose result depends on caller values. It is assigned only by the lexical
// binding authority; consumers must never reconstruct it from a callee name.
type Intrinsic uint8

const (
	IntrinsicNone Intrinsic = iota
	IntrinsicLuaType
)

// Valid reports whether the intrinsic identifies a registered operation.
func (k Intrinsic) Valid() bool { return k > IntrinsicNone && k <= IntrinsicLuaType }
