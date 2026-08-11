// Package evaluation owns the Seal-local evaluation-order projection.
//
// It walks the typed authored expression and direct value-consuming statement
// relations using the language's evaluation laws. Branch/Loop topology and
// their body phases belong to recurrence and are deliberately not roots here.
// The Session retains no graph, sidecar, or source-order authority. Its only
// output is the ordered sequence of authored Select occurrences reachable from
// one root.
package evaluation
