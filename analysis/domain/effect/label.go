package effect

// Label is one atomic function effect.
type Label interface {
	EffectLabel()
	String() string
	Equals(other Label) bool
}
