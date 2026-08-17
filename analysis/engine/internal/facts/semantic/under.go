package semantic

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
)

// EqualUnder proves exact equality over one supplied shared support. Scratch
// belongs to the caller's typed evaluator work so Domain remains immutable
// semantic authority rather than retaining scheduler-owned mutable state.
func (domain *Domain[F, K, V]) EqualUnder(left, right Plane[F, K, V], within support.Mask, scratch *diagram.SoleScratch[K, V]) bool {
	DbgEqualUnder.Add(1)
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSupport(within) {
		return false
	}
	return domain.relateUnder(left, right, within, scratch, domain.ops.Equal)
}

// LessOrEqUnder proves typed pointwise order over one supplied support using
// caller-owned typed traversal storage.
func (domain *Domain[F, K, V]) LessOrEqUnder(left, right Plane[F, K, V], within support.Mask, scratch *diagram.SoleScratch[K, V]) bool {
	DbgLessOrEqUnder.Add(1)
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSupport(within) {
		return false
	}
	return domain.relateUnder(left, right, within, scratch, domain.ops.LessOrEq)
}

func (domain *Domain[F, K, V]) relateUnder(left, right Plane[F, K, V], within support.Mask, scratch *diagram.SoleScratch[K, V], relation func(V, V) bool) bool {
	if relation == nil || scratch == nil {
		return false
	}
	// Equality and pointwise order are reflexive. This semantic-only fast path
	// is deliberately above Diagram's generic callback API: Diagram must still
	// call an arbitrary structural visitor for identical roots.
	if left.root == right.root {
		return true
	}
	return domain.diagram.CompareSoleFactorUnder(left.root, right.root, within, scratch, func(first, second terminal.ID[V]) bool {
		leftValue, leftOK := domain.ops.Default, true
		if first != (terminal.ID[V]{}) {
			leftValue, leftOK = domain.terminals.Value(first)
		}
		rightValue, rightOK := domain.ops.Default, true
		if second != (terminal.ID[V]{}) {
			rightValue, rightOK = domain.terminals.Value(second)
		}
		return leftOK && rightOK && relation(leftValue, rightValue)
	})
}
