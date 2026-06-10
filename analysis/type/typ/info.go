package typ

// equaler is implemented by types that support typed equality comparison.
type equaler interface {
	Equals(other any) bool
}

// EffectInfo describes function effects (e.g., io, throw).
// Implemented by effect.Row.
type EffectInfo interface {
	equaler
	IsEffectInfo()
}
