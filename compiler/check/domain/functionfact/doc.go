// Package functionfact owns the per-function fact abstract domain.
//
// It canonicalizes and joins api.FunctionFact values, constructs canonical
// FunctionFacts maps from per-symbol evidence, and owns store-backed projection
// of function-fact types. Product-level packages decide when facts are produced;
// this package decides what the fact product means.
package functionfact
