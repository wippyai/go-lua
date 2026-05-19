// Package functionfact owns the per-function fact abstract domain.
//
// It canonicalizes and joins one api.FunctionFact at a time: parameter
// evidence, return summaries, narrow summaries, and the projected function type.
// Product-level packages decide when facts are read from or written to maps;
// this package decides what one function fact means and how it combines.
package functionfact
