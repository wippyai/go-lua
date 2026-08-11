// Package cutplan defines the fail-closed, high-level input to flashrefactor.
//
// A cut plan is intentionally not a programming language for semantics.  It
// describes an already-reviewed ownership cut, exact read/write footprint,
// and verification obligations. Nothing in this package discovers ownership,
// runs shell commands, expands globs, accepts an implicit compatibility
// surface, or stores generated evidence inside Intent.
//
// The workflow is:
//
//  1. author a complete Intent;
//  2. validate and canonically digest it;
//  3. obtain resolver/reference evidence and byte fingerprints separately;
//  4. bind both to a Lock; and
//  5. make a mechanical executor accept only that Lock.
//
// This makes an incomplete move fail before it can turn into an adapter or a
// second implementation path.
package cutplan
