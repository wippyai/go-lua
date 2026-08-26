// Package differential owns the signed before/after transport for semantic
// Apply results.
//
// A Differential retains the original apply.Application values, including
// their proposal leases and authenticated lineage.  It does not rebuild an
// application from proposal cells, copy a lease into a second batch, or
// infer a side from the other one.  A side may be omitted by passing the zero
// apply.Application value; at least one authenticated side is required.
//
// This package is transport only.  It does not classify destinations,
// transact state, publish proposals, or evaluate an Apply expression.
package differential
