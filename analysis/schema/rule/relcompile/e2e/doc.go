// Package e2e is the W2 exit demo: one Lua fixture carried across every
// authority of the relational engine, from the artifact its source compiles to
// down to the converged relations a reference evaluator would answer.
//
// The demo is a gate, not a survey. It names one fixture, the family chain its
// answer is derived by, and one stage per authority, so a failure names the
// exact authority the chain stops at instead of a diffuse end-to-end
// mismatch. The stages are the authority chain of the design:
//
//	relcompile.Compile -> check.Check -> mount.Specialize -> solve
//
// The declaration surfaces the demo installs are its own, and they are
// authored to be admissible to the checker rather than only to the compiler:
// one value lattice per axis, one execution schema identity carried by every
// sealed signature, and signature inputs that name the relation the lowered
// expression delivers. That is what makes the demo an engine measurement
// instead of a harness measurement - a refusal here is a statement about the
// engine, because the declaration side of it is already correct.
//
// The stages beyond the checker are not written yet, because the surfaces they
// would consume do not exist:
//
//   - mount.Specialize requires one binding.ValueAlgebra per certified TypeID.
//     The value axis's ascent authority is domain/value/relation.ValueLattice
//     over a sealed *value.Schema, and no compiled artifact publishes that
//     schema, so a mount inventory cannot be assembled from a fixture.
//   - internal/relationoracle exposes relational operators, not an evaluator
//     over a mounted plan. There is no call that carries a mounted certificate
//     to a fixpoint, so the solve stage has no entry point to name.
package e2e
