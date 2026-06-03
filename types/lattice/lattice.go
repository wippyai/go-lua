// Package lattice defines the abstract-domain contract for every abstract
// interpreter in the type system.
//
// Every abstract domain — Condition, AbstractValue, NumericRange,
// PathPresence, TypeNarrowing — exposes a Lattice value. The contract is the
// algebraic foundation for the fixed-point algorithms in the flow engine and
// propagate.Propagate: a domain that satisfies the laws (idempotency,
// commutativity, associativity, monotonicity, ACC under widening) makes those
// algorithms terminate with a sound over-approximation of the collecting
// semantics. A domain that does not satisfy the laws does not — and the law
// harness in this package catches the violation as a unit-test failure rather
// than as a fixture hang.
//
// Lattice is a struct of function fields, not an interface. The user has
// explicitly forbidden adapter types; each domain exposes its operations as a
// Lattice value built from package-level constructors (Apron-style), no
// wrapper struct. See types/constraint/condition_lattice.go for the canonical
// example.
//
// Domains with finite height (numeric ranges, presence, length bounds) set
// Widen to Join. Domains without finite height (Condition-DNF over an
// unbounded literal set, recursive records) must provide a true Cousot
// widening: an upper bound such that any ascending chain under repeated Widen
// application is eventually stationary.
//
// Reference: Patrick Cousot & Radhia Cousot, "Abstract interpretation: a
// unified lattice model for static analysis of programs by construction or
// approximation of fixpoints", POPL 1977.
//
// Per-domain widening rationale is documented at the domain (see
// types/constraint/DOMAIN_DESIGN.md for Condition).
package lattice

// Lattice is the abstract-domain contract, as a struct of function fields.
//
// Each field implements one algebraic operation. LawSuite calls these fields
// directly. Domain authors construct a Lattice value by setting each field to
// the appropriate package-level function; there is no implementing type.
//
// Field contracts:
//
//   - Bottom: least element. Bottom() ⊑ x for all x.
//   - Top: greatest element. x ⊑ Top() for all x.
//   - Equal: reflexive, symmetric, transitive equality on the carrier.
//   - LessOrEq: a ⊑ b in the partial order — reflexive, transitive,
//     antisymmetric with Equal.
//   - Join: least upper bound — commutative, associative, idempotent,
//     monotone.
//   - Meet: greatest lower bound — commutative, associative, idempotent,
//     monotone. May be nil for forward-only domains whose analyzer surface
//     does not consume a greatest lower bound (e.g. AbstractValue);
//     LawSuite skips the meet-side laws (including absorption) when nil.
//   - Widen: Cousot widening operator —
//     prev ⊑ Widen(prev, next), next ⊑ Widen(prev, next), and for any
//     monotone f: L → L, the sequence s₀ = ⊥, sᵢ₊₁ = Widen(sᵢ, f(sᵢ)) is
//     eventually stationary.
type Lattice[T any] struct {
	Bottom   func() T
	Top      func() T
	Equal    func(a, b T) bool
	LessOrEq func(a, b T) bool
	Join     func(a, b T) T
	Meet     func(a, b T) T
	Widen    func(prev, next T) T
}
