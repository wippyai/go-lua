package cfgbuild

// SourceSpan is a syntax-free source range carried by source facts for
// downstream consumers that must not inspect AST nodes.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}
