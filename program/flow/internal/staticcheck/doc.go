// Package staticcheck is Flow's final Static receipt check.
//
// It joins the already sealed Source, authored Flow, Static, Body, Binding,
// containment, and direct-binding proofs.  The package intentionally returns
// only static.CommitInput: its lexical context and all validation scratch die
// with Validate, and no second Static authority is retained.
package staticcheck
