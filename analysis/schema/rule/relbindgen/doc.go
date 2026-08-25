// Package relbindgen is the thin typed binding layer between owner domain
// mathematics and the generic relational runtime.
//
// Go requires exactly one boundary between heterogeneous domain payload types
// and generic runtime storage. This package is that boundary and nothing else.
// It supplies two artifacts, and a future generator emits only these two per
// family:
//
//  1. a thin typed semantic-operation binding: BindScalar, BindExpansion,
//     BindReduction and BindUpdate turn one sealed signature.Signature plus one
//     owner operation into a binding.Factory;
//  2. a thin typed owner-column publisher: Column carries one TypeID's codec
//     over a solve-local Store, and Algebra states that TypeID's ascent
//     mathematics as a binding.ValueAlgebra.
//
// Nothing here reads a relation, joins, routes, schedules, tickets, settles an
// outcome, selects a form, or choreographs publication. Those belong to the
// relational engine.
//
// An owner operation is stated as a Go type whose method signatures mention
// only decoded domain values, a borrowed Span, and a bounded Emitter. No
// relation, cell token, proposal buffer, scope, witness, issuer or engine
// value is in scope inside an operation, so a binding that reaches outside its
// frame is not merely rejected at runtime: it does not compile, because there
// is no identifier through which the reach could be written.
package relbindgen
