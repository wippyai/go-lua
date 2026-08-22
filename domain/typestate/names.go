package typestate

// ProtocolFromString returns a user-defined protocol name. Empty protocol names
// are invalid because resources are keyed by protocol namespace.
func ProtocolFromString(name string) (Protocol, bool) {
	if name == "" {
		return "", false
	}
	return Protocol(name), true
}

func (p Protocol) String() string { return string(p) }

// StateFromString returns a user-defined protocol state. Empty state names are
// invalid when a caller requires a concrete state.
func StateFromString(name string) (State, bool) {
	if name == "" {
		return "", false
	}
	return State(name), true
}

func (s State) String() string { return string(s) }
