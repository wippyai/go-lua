// Package formal defines the neutral vocabulary shared by finite relational
// domains. It is deliberately below value axes, state, and transformer code.
package formal

// Vocabulary selects one side of a finite relational schema.
type Vocabulary uint8

const (
	Invalid Vocabulary = iota
	Input
	Middle
	Output
)

// Valid reports whether v names a declared relational vocabulary.
func (v Vocabulary) Valid() bool {
	return v >= Input && v <= Output
}

// String returns the stable diagnostic spelling of v. Semantic identity is
// always the typed enum value, never this presentation string.
func (v Vocabulary) String() string {
	switch v {
	case Input:
		return "in"
	case Middle:
		return "mid"
	case Output:
		return "out"
	default:
		return "invalid"
	}
}
