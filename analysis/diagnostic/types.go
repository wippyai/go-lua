package diagnostic

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/source"
)

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

// Position identifies a source location.
type Position = source.Position

// Span defines a source range.
type Span = source.Span

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
	Explanation Explanation
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
