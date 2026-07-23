package factflow

// SourceSpan is a syntax-free source range carried by lowered facts for later
// diagnostics and obligations. It intentionally contains positions only, not
// AST nodes.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

func copySourceSpans(in []SourceSpan) []SourceSpan {
	if len(in) == 0 {
		return nil
	}
	return append([]SourceSpan(nil), in...)
}
