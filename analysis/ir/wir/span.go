package wir

// Span is source-coordinate metadata attached to lowered WIR for diagnostics.
// It is deliberately a scalar value owned by WIR so the IR does not import a
// frontend source package. Lowering adapts parser spans into this shape;
// transfer/rendering adapts it out to factflow or diagnostics as needed.
type Span struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Valid reports whether this span has source coordinates.
func (s Span) Valid() bool {
	return s.StartLine > 0
}
