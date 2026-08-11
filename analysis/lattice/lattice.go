// Package lattice owns the minimal finite-height lattice interface used by the
// solver and domain packages; concrete domains live elsewhere.
//
// Every abstract domain exposes a Lattice value. The contract is the algebraic
// foundation for fixed-point algorithms: a domain that satisfies the laws
// (idempotency, commutativity, associativity, monotonicity, and sound widening)
// makes those algorithms converge to a sound over-approximation of the
// collecting semantics. Sampled domain laws can falsify this contract; the
// Factor boundary validates the executable termination witness on every
// actual widening or narrowing transition.
//
// Lattice is a struct of function fields, not an interface. Each domain
// exposes its operations as a Lattice value built from package-level
// constructors (Apron-style), no wrapper struct.
//
// Domains with finite height set Widen to Join. Domains without finite height
// must provide a true Cousot widening: an upper bound such that any ascending
// chain under repeated Widen application is eventually stationary. The
// key-aware executable descent witness for that statement belongs at the
// Factor boundary, where the sealed coordinate universe is known; a sampled
// lattice-law harness cannot prove termination by running a capped chain.
//
// Reference: Patrick Cousot & Radhia Cousot, "Abstract interpretation: a
// unified lattice model for static analysis of programs by construction or
// approximation of fixpoints", POPL 1977.
package lattice

// Lattice is the abstract-domain contract, as a struct of function fields.
//
// Each field implements one algebraic operation. Domain authors construct a
// Lattice value by setting each field to the appropriate package-level
// function; there is no implementing type.
//
// Field contracts:
//
//   - Every operation is pure and deterministic. It must neither mutate an
//     input nor return a mutable alias that can later alter an already
//     published abstract value. Values are persistent immutable snapshots;
//     this is required for structural sharing, cache safety, and reproducible
//     fixed-point proofs.
//   - Bottom: least element. Bottom() ⊑ x for all x.
//   - Top: greatest element. x ⊑ Top() for all x.
//   - Equal: reflexive, symmetric, transitive equality on the carrier.
//   - Same: optional representation-identity predicate. When present, it may
//     return true only for values that are known Equal, and lets persistent
//     domains avoid re-comparing a shared immutable representation.
//   - LessOrEq: a ⊑ b in the partial order — reflexive, transitive,
//     antisymmetric with Equal.
//   - Join: least upper bound — commutative, associative, idempotent,
//     monotone.
//   - Meet: greatest lower bound — commutative, associative, idempotent,
//     monotone. May be nil for forward-only domains whose analyzer surface
//     does not consume a greatest lower bound; law tests skip the meet-side
//     laws (including absorption) when nil.
//   - Widen: Cousot widening operator —
//     prev ⊑ Widen(prev, next), next ⊑ Widen(prev, next), and for any
//     monotone f: L → L, the sequence s₀ = ⊥, sᵢ₊₁ = Widen(sᵢ, f(sᵢ)) is
//     eventually stationary.
//   - Narrow: optional Cousot narrowing operator for a well-founded decreasing
//     phase after widening has stabilized. Narrow(prev, next) must stay an
//     over-approximation of next and be no more precise than facts justified by
//     next; domains may leave it nil to keep the widened value unchanged.
type Lattice[T any] struct {
	Bottom   func() T
	Top      func() T
	Equal    func(a, b T) bool
	Same     func(a, b T) bool
	LessOrEq func(a, b T) bool
	Join     func(a, b T) T
	Meet     func(a, b T) T
	Widen    func(prev, next T) T
	Narrow   func(prev, next T) T
}
