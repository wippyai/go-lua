package parsersource

// FieldState is one representation state an AST field can hold. The vocabulary
// is closed and structural: it distinguishes the states a field's declared form
// can be in, not the semantic values it can carry. A field of a given form can
// only ever be in one of the states its form admits, so the pair (form, state)
// is a complete state space rather than a sample of one.
type FieldState uint8

const (
	FieldStateInvalid FieldState = iota
	FieldStateAbsent
	FieldStatePresent
	FieldStateEmpty
	FieldStateNonEmpty
	FieldStateFalse
	FieldStateTrue
	FieldStateZero
	FieldStateNonZero
)

// String is the stable spelling used wherever a state has to appear inside a
// row key. It is a wire spelling, so it is written out here rather than derived
// from the constant name.
func (s FieldState) String() string {
	switch s {
	case FieldStateAbsent:
		return "absent"
	case FieldStatePresent:
		return "present"
	case FieldStateEmpty:
		return "empty"
	case FieldStateNonEmpty:
		return "non-empty"
	case FieldStateFalse:
		return "false"
	case FieldStateTrue:
		return "true"
	case FieldStateZero:
		return "zero"
	case FieldStateNonZero:
		return "non-zero"
	default:
		return "invalid"
	}
}

// States is the complete state space of a field of this form, in a stable
// order. It is derived from the declared form alone: a pointer or interface
// child is either absent or present, a slice or string is either empty or
// carries elements, a bool is either false or true, and a scalar or named value
// is either its zero or some other value. A form with no state space returns
// nothing rather than a single catch-all state, so a caller cannot mistake an
// unmodelled form for a fully enumerated one.
func (f FieldForm) States() []FieldState {
	switch f {
	case FieldFormOptional, FieldFormMapping, FieldFormInterface:
		return []FieldState{FieldStateAbsent, FieldStatePresent}
	case FieldFormSequence, FieldFormString:
		return []FieldState{FieldStateEmpty, FieldStateNonEmpty}
	case FieldFormBool:
		return []FieldState{FieldStateFalse, FieldStateTrue}
	case FieldFormNamed, FieldFormScalar:
		return []FieldState{FieldStateZero, FieldStateNonZero}
	default:
		return nil
	}
}
