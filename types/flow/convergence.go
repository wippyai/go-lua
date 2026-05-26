package flow

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// sameFlowValue reports whether two flow facts are the same point in the
// value-domain fixpoint lattice. It compares product identity (product.Equal)
// over the AbstractValue carrier, not SameConvergedFact over projected typ.Type:
// the product interner collapses converged recursive families to one canonical
// node, so a self-similar growth pattern reaches a fixed point here where a
// node-exact typ.Type relation never does. This is the worklist no-op / change
// detection; exact query/input equality stays in equal.go.
func sameFlowValue(a, b typ.Type) bool {
	return product.Equal(liftFlowValue(a), liftFlowValue(b))
}

// sameFlowValueAV compares two stored carriers directly, the native form of the
// store-convergence relation with no admission re-lift.
func sameFlowValueAV(a, b product.AbstractValue) bool {
	return a.Equal(b)
}

func sameFlowValueMap(a, b map[string]product.AbstractValue) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !sameFlowValueAV(av, bv) {
			return false
		}
	}
	return true
}

func sameFlowValueVector(a, b []typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameFlowValue(a[i], b[i]) {
			return false
		}
	}
	return true
}
