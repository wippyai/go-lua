// Package functionfact owns the FunctionFacts projection domain.
//
// It canonicalizes and joins api.FunctionFact values, constructs final/public
// FunctionFacts maps from per-symbol evidence, and owns store-backed projection
// of function-fact types, parameter evidence, and return summaries. Canonical
// Summary decides semantic truth; this package decides what the projection
// product means.
package functionfact
