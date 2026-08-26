// Package relationconstructor carries one sealed compilation into the
// relation engine.
//
// It sits directly above relationadmission. Admission composes an authored
// relation declaration with owner-supplied runtime authorities into one
// ready-to-solve root; this package is what authors that declaration from a
// sealed compilation and supplies those authorities. It compiles no rule of
// its own, seals no domain schema, and reimplements no certificate, mount,
// state, or snapshot authority.
//
// Construction is parameterized by one Composition: the exact set of declared
// rules a construction admits, stated before the declaration is sealed. A
// rule the composition does not admit is absent from the sealed declaration
// rather than present and inert, so the schema states exactly what the caller
// asked for and no downstream authority observes a rule that was never meant
// to participate.
package relationconstructor
