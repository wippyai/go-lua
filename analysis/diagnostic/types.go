package diagnostic

import "fmt"

// Severity classifies how strongly a diagnostic should affect the caller.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityHint
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// Position identifies a source location. Line and Column are 1-indexed.
type Position struct {
	File   string
	Line   int
	Column int
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

// Code identifies a diagnostic family. Producers own their code namespace.
type Code string

func (c Code) String() string {
	if c == "" {
		return "diagnostic"
	}
	return string(c)
}

// Diagnostic is the analysis-facing diagnostic value model.
type Diagnostic struct {
	Position    Position
	Span        Span
	Code        Code
	Message     string
	Severity    Severity
	Explanation string
	Help        string
	Labels      []Label
}

// Label marks a secondary source location with an annotation message.
type Label struct {
	Span    Span
	Message string
}

func (d Diagnostic) String() string {
	if d.Position.Valid() {
		return fmt.Sprintf("%s: %s", d.Position, d.Message)
	}
	return d.Message
}

// Error implements the error interface.
func (d Diagnostic) Error() string {
	return d.String()
}
