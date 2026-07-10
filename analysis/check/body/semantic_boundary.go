package body

// SourceSpan is a syntax-free source range carried by check/body source facts.
// Consumers above body depend on this type instead of lower CFG construction
// packages.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}
