package effect

// Throw indicates that a function may raise an error.
type Throw struct{}

func (Throw) label()         {}
func (Throw) String() string { return "throw" }
func (Throw) Equals(other Label) bool {
	_, ok := other.(Throw)
	return ok
}

// Diverge indicates that a function may not terminate.
type Diverge struct{}

func (Diverge) label()         {}
func (Diverge) String() string { return "diverge" }
func (Diverge) Equals(other Label) bool {
	_, ok := other.(Diverge)
	return ok
}

// IO indicates that a function performs I/O.
type IO struct{}

func (IO) label()         {}
func (IO) String() string { return "io" }
func (IO) Equals(other Label) bool {
	_, ok := other.(IO)
	return ok
}
