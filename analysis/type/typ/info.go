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

// SpecInfo describes function specifications (pre/post conditions).
// Implemented by *contract.Spec.
type SpecInfo interface {
	equaler
	IsSpecInfo()
}

// RefinementInfo describes type refinements from function calls.
// Implemented by *constraint.FunctionRefinement.
type RefinementInfo interface {
	equaler
	IsRefinementInfo()
}
