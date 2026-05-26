package value

import (
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// SameConvergedFact reports whether two stored value facts are the same point
// in the value-domain convergence lattice. Solvers use this for no-op/change
// detection after joins and widening. Query input equality must remain exact;
// this relation is only for already-normalized abstract state.
func SameConvergedFact(a, b typ.Type) bool {
	if sameValueNodeOrAcyclicEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !typ.ContainsRecursive(a) && !typ.ContainsRecursive(b) {
		return false
	}
	return sameRecursiveConvergedFact(a, b)
}

// Covers reports whether upper admits observation in the value-domain evidence
// lattice. It is the public boundary for callers that would otherwise reach for
// generic subtype/equality while merging abstract evidence. Recursive products
// are handled by product-family coverage; acyclic products use ordinary subtype.
func Covers(upper, observation typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(upper, observation) {
		return true
	}
	if upper == nil || observation == nil {
		return false
	}
	if typ.ContainsRecursive(upper) || typ.ContainsRecursive(observation) {
		// Same-family recursive products are the same point in the convergence
		// lattice, so each covers the other; the directional self-embedding
		// relation only admits strictly-extending observations.
		if SameConvergedFact(upper, observation) {
			return true
		}
		return RecursiveEvidenceCovers(upper, observation)
	}
	return subtype.IsSubtype(observation, upper)
}

// sameValueNodeOrAcyclicEqual is the value-domain equality guard for hot
// evidence and convergence relations. Recursive products are compared by their
// explicit owner relations, not by generic structural hashing.
func sameValueNodeOrAcyclicEqual(a, b typ.Type) bool {
	return typ.SameNodeOrAcyclicEqual(a, b)
}

func sameRecursiveConvergedFact(a, b typ.Type) bool {
	// Convergence equality over a recursive product is canonical-family identity:
	// both observations hash-cons to one canonical representative through the
	// metadata-sensitive verifier (CanonicalRecursiveFamily), so the same family
	// is the same node pointer. This is the kernel of the family hash by
	// construction and never re-walks the structure or unfolds the cycle on each
	// solver no-op check.
	return CanonicalRecursiveFamily(a) == CanonicalRecursiveFamily(b)
}
