package diagnostic

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/embedding"
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
	// Location is the digest-bound semantic location. Position.File and label
	// File fields are display projections retained for terminal fixtures and
	// compatibility with existing renderers.
	Location    embedding.SourceLocation
	Position    Position
	Span        Span
	Code        Code
	Message     string
	Severity    Severity
	Explanation Explanation
	Help        string
	Labels      []Label
}

// DiagnosticSpec is the canonical constructor input for analysis diagnostics.
// Producers should provide the semantic fields and the primary span; New keeps
// Position in sync with Span for terminal, LSP, and test consumers.
type DiagnosticSpec struct {
	Location    embedding.SourceLocation
	File        string
	Span        Span
	Code        Code
	Message     string
	Severity    Severity
	Explanation Explanation
	Help        string
	Labels      []Label
}

// New builds a Diagnostic with Position derived from the primary span.
func New(spec DiagnosticSpec) Diagnostic {
	position := PositionFromSpan(spec.Span)
	position.File = spec.File
	return Diagnostic{
		Location:    spec.Location,
		Position:    position,
		Span:        spec.Span,
		Code:        spec.Code,
		Message:     spec.Message,
		Severity:    spec.Severity,
		Explanation: spec.Explanation,
		Help:        spec.Help,
		Labels:      append([]Label(nil), spec.Labels...),
	}
}

// PositionFromSpan returns the primary cursor position for a source span.
func PositionFromSpan(span Span) Position {
	return Position{
		Line:      span.StartLine,
		Column:    span.StartCol,
		EndLine:   span.EndLine,
		EndColumn: span.EndCol,
	}
}

// PositionFromSpanInFile returns the primary cursor position for a source span
// in file.
func PositionFromSpanInFile(file string, span Span) Position {
	pos := PositionFromSpan(span)
	pos.File = file
	return pos
}

// Label marks a secondary source location with an annotation message.
type Label struct {
	Location embedding.SourceLocation
	// Deprecated: display projection only.
	File      string
	Span      Span
	Message   string
	Placement LabelPlacement
}

// LabelPlacement controls where rich source-frame labels render relative to
// the source line. Auto keeps the structural fallback: primary annotations below
// the line, secondary annotations above it.
type LabelPlacement uint8

const (
	LabelPlacementAuto LabelPlacement = iota
	LabelPlacementAbove
	LabelPlacementBelow
)

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
