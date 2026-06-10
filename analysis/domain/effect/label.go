package effect

// Label is one atomic function effect.
type Label interface {
	label()
	String() string
	Equals(other Label) bool
}
