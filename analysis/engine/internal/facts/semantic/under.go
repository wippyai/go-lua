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
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSupport(within) {
		return false
	}
	return domain.relateUnder(left, right, within, scratch, domain.ops.Equal)
}

// LessOrEqUnder proves typed pointwise order over one supplied support using
// caller-owned typed traversal storage.
func (domain *Domain[F, K, V]) LessOrEqUnder(left, right Plane[F, K, V], within support.Mask, scratch *diagram.SoleScratch[K, V]) bool {
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSupport(within) {
		return false
	}
	return domain.relateUnder(left, right, within, scratch, domain.ops.LessOrEq)
}

// relateUnder proves one reflexive typed relation cellwise. Reflexivity is a
// precondition of both callers, and it is what licenses the two identity fast
// paths below: one whole root, one terminal cell. Both are semantic-only and
// deliberately above Diagram's generic callback API, which must still call an
// arbitrary structural visitor.
func (domain *Domain[F, K, V]) relateUnder(left, right Plane[F, K, V], within support.Mask, scratch *diagram.SoleScratch[K, V], reflexive func(V, V) bool) bool {
	if reflexive == nil || scratch == nil {
		return false
	}
	if left.root == right.root {
		return true
	}
	return domain.diagram.CompareSoleFactorUnder(left.root, right.root, within, scratch, func(first, second terminal.ID[V]) bool {
		// Publication canonicalizes equal values onto one terminal identity, so
		// an identical cell needs no value read and no typed relation call.
		if first == second {
			return true
		}
		leftValue, leftOK := domain.ops.Default, true
		if first != (terminal.ID[V]{}) {
			leftValue, leftOK = domain.terminals.Value(first)
		}
		rightValue, rightOK := domain.ops.Default, true
		if second != (terminal.ID[V]{}) {
			rightValue, rightOK = domain.terminals.Value(second)
		}
		return leftOK && rightOK && reflexive(leftValue, rightValue)
	})
}
