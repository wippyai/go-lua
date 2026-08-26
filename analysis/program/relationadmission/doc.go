// Package relationadmission composes an already-authored relation declaration
// with its owner-supplied runtime authorities into one ready-to-solve root.
//
// It is deliberately above both relcompile and the relation engine.  The
// package does not author rules, interpret domain values, or reimplement
// certificate, mount, state, or snapshot authority.  It only admits their
// sealed outputs in the required order.
package relationadmission
