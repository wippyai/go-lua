// Package astcodec contains generated, typed encoders for compiler/ast values.
//
// The generated implementation is deliberately kept separate from the
// parser and from grammarproof's semantic requirements. It is cold proof
// support: callers may observe AST values, but no parser or Program path uses
// this package to construct semantics.
package astcodec

// FieldState is the closed state vocabulary used by grammarproof traces.
// Values are kept independent of the parent proof package so this codec does
// not need to import it (which would create an import cycle).
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

// Field is one exact exported AST field. Value is retained only for the
// closed signed/unsigned scalar discriminants consumed by grammarproof;
// positions, interfaces, and other forms intentionally carry zero payload.
type Field struct {
	Name  string     `json:"name"`
	State FieldState `json:"state"`
	Value uint64     `json:"value,omitempty"`
}

// Occurrence is one concrete AST value and its exported-field observation.
type Occurrence struct {
	Type      string  `json:"type"`
	StartLine int     `json:"start_line"`
	StartCol  int     `json:"start_col"`
	EndLine   int     `json:"end_line"`
	EndCol    int     `json:"end_col"`
	Fields    []Field `json:"fields"`
}
