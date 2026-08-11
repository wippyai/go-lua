// Package semantic collects the resolver evidence a flash-refactor lock needs.
//
// It deliberately has no rewrite operation and does not infer ownership.  A
// reviewed cut supplies exact object locations and source paths; this package
// resolves them through go/packages and go/types with one disposable cache,
// normalizes the result into cutplan coordinates, and rejects incomplete or
// stale evidence.
package semantic
