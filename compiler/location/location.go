// Package location defines source-code coordinates shared by compiler leaves.
package location

import "fmt"

// Position identifies a source location. Line and Column are 1-indexed.
type Position struct {
	File      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
}

// Valid reports whether the position contains a concrete editor location.
func (p Position) Valid() bool {
	return p.Line > 0 && p.Column > 0
}

func (p Position) String() string {
	if p.File == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Column)
	}
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
}

// Span defines a source range. All fields are 1-indexed; zero end fields mean
// the extent is unknown or point-like.
type Span struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Valid returns true if the span has meaningful positions.
func (s Span) Valid() bool {
	return s.StartLine > 0 && s.StartCol > 0
}

// SingleLine returns true if the span does not cross line boundaries.
func (s Span) SingleLine() bool {
	return s.StartLine == s.EndLine || s.EndLine == 0
}
