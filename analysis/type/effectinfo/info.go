package effectinfo

// Equaler is implemented by effect payloads that support typed equality.
type Equaler interface {
	Equals(other any) bool
}

// Info is an opaque function-effect payload stored on function types.
type Info interface {
	Equaler
	IsEffectInfo()
}
