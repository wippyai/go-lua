// Package lattice defines the abstract-domain contract for every abstract
// interpreter in the type system.
//
// Every abstract domain — Condition, AbstractValue, NumericRange, LengthBound,
// PathPresence, TypeNarrowing — implements the Lattice interface. The contract
// is the algebraic foundation for the fixed-point algorithms in flow.Solve and
// propagate.Propagate: a domain that satisfies the laws (idempotency,
// commutativity, associativity, monotonicity, ACC under widening) makes those
// algorithms terminate with a sound over-approximation of the collecting
// semantics. A domain that does not satisfy the laws does not — and the law
// harness in this package catches the violation as a unit-test failure rather
// than as a fixture hang.
//
// The interface is intentionally minimal. Domains with finite height (numeric
// ranges, presence, length bounds) implement Widen as Join. Domains without
// finite height (Condition-DNF over an unbounded literal set, recursive
// records) must provide a true Cousot widening: an upper bound such that any
// ascending chain under repeated Widen application is eventually stationary.
//
// Reference: Patrick Cousot & Radhia Cousot, "Abstract interpretation: a
// unified lattice model for static analysis of programs by construction or
// approximation of fixpoints", POPL 1977.
package lattice

// Lattice is the abstract-domain contract.
//
// Implementations must satisfy the algebraic laws asserted by AssertLaws.
// A domain that fails a law cannot be used as an abstract interpreter
// without losing soundness or termination — both are required.
type Lattice[T any] interface {
	// Bottom is the least element. Bottom() ⊑ x for all x.
	// Represents "no information" / the empty concretization.
	Bottom() T

	// Top is the greatest element. x ⊑ Top() for all x.
	// Represents "any information" / the full concretization.
	Top() T

	// Equal reports whether a and b denote the same lattice element.
	// Must be reflexive, symmetric, transitive.
	Equal(a, b T) bool

	// LessOrEq reports whether a ⊑ b in the partial order.
	// Must be reflexive, transitive, and antisymmetric with Equal.
	LessOrEq(a, b T) bool

	// Join is the least upper bound: a ⊑ Join(a,b) ⊒ b.
	// Must be commutative, associative, idempotent, monotone.
	Join(a, b T) T

	// Meet is the greatest lower bound: a ⊒ Meet(a,b) ⊑ b.
	// Must be commutative, associative, idempotent, monotone.
	Meet(a, b T) T

	// Widen is a Cousot widening operator.
	//
	// Properties:
	//
	//   prev ⊑ Widen(prev, next)          — over-approximates prev
	//   next ⊑ Widen(prev, next)          — over-approximates next
	//   for every monotone f: L → L,
	//     the sequence s₀ = ⊥, sᵢ₊₁ = Widen(sᵢ, f(sᵢ))
	//     is eventually stationary                                 — termination
	//
	// A domain with finite ascending chains (finite height) may implement
	// Widen as Join. A domain without finite height must implement a true
	// widening — see Cousot–Cousot POPL 1977.
	Widen(prev, next T) T
}
