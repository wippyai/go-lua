// Package product builds lattice combinators over the lattice.Lattice
// contract: a product-of-lattices and a pointwise map lattice.
//
// These combinators are the substrate for a single-canonical-flow refactor.
// The per-program-point abstract state is a PRODUCT of the existing axis
// lattices (Condition, AbstractValue, NumericRange, PathPresence, ...), and
// the per-symbol environment is a MAP lattice keyed by symbol. Because the
// combinators are derived structurally from the component lattices, the
// product and the map satisfy the full lattice laws exactly when their
// components do; domain law tests verify this against any component
// instantiation.
//
// Componentwise constructions preserve the algebraic laws by a standard
// argument: each operation is applied independently per component, so
// idempotency, commutativity, associativity, monotonicity, and the
// ascending-chain condition under widening all lift from the components. A
// product's height is the sum of component heights, so a componentwise
// widening built from terminating component widenings terminates.
//
// Reference: Patrick Cousot & Radhia Cousot, "Abstract interpretation: a
// unified lattice model for static analysis of programs by construction or
// approximation of fixpoints", POPL 1977 — §6 on Cartesian (reduced) products.
package product

import "github.com/wippyai/go-lua/analysis/domain/lattice"

// Pair is the carrier of a two-component product lattice. A and B are the two
// component values; the partial order, lattice operations, and widening are
// all componentwise.
type Pair[A, B any] struct {
	A A
	B B
}

// Product2 builds the componentwise product of two lattices.
//
// Bottom and Top are the componentwise least and greatest elements. Equal,
// LessOrEq, Join, and Widen apply the corresponding component operation to
// each component independently. Meet is provided only when BOTH components
// provide a Meet; if either component is forward-only (Meet == nil) the
// product's Meet is nil, matching the contract that a product with a
// forward-only component is itself forward-only. Law tests then skip the
// meet-side laws for the product, exactly as they do for the component.
func Product2[A, B any](la lattice.Lattice[A], lb lattice.Lattice[B]) lattice.Lattice[Pair[A, B]] {
	l := lattice.Lattice[Pair[A, B]]{
		Bottom: func() Pair[A, B] {
			return Pair[A, B]{A: la.Bottom(), B: lb.Bottom()}
		},
		Top: func() Pair[A, B] {
			return Pair[A, B]{A: la.Top(), B: lb.Top()}
		},
		Equal: func(x, y Pair[A, B]) bool {
			return la.Equal(x.A, y.A) && lb.Equal(x.B, y.B)
		},
		LessOrEq: func(x, y Pair[A, B]) bool {
			return la.LessOrEq(x.A, y.A) && lb.LessOrEq(x.B, y.B)
		},
		Join: func(x, y Pair[A, B]) Pair[A, B] {
			return Pair[A, B]{A: la.Join(x.A, y.A), B: lb.Join(x.B, y.B)}
		},
		Widen: func(prev, next Pair[A, B]) Pair[A, B] {
			return Pair[A, B]{A: la.Widen(prev.A, next.A), B: lb.Widen(prev.B, next.B)}
		},
	}
	// Meet lifts only when both components define it; otherwise the product is
	// forward-only and Meet stays nil so law tests skip the meet-side laws.
	if la.Meet != nil && lb.Meet != nil {
		l.Meet = func(x, y Pair[A, B]) Pair[A, B] {
			return Pair[A, B]{A: la.Meet(x.A, y.A), B: lb.Meet(x.B, y.B)}
		}
	}
	return l
}

// Product3 nests two Product2 applications into a single right-associated
// product. It is sugar over Product2(la, Product2(lb, lc)); the carrier is
// Pair[A, Pair[B, C]]. Meet-nil propagation follows from Product2: the result
// has a Meet only when all three components do.
func Product3[A, B, C any](
	la lattice.Lattice[A],
	lb lattice.Lattice[B],
	lc lattice.Lattice[C],
) lattice.Lattice[Pair[A, Pair[B, C]]] {
	return Product2(la, Product2(lb, lc))
}
