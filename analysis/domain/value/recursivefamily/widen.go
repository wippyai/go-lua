package recursivefamily

import (
	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Widen widens the body slot of a family handle by joining the candidate
// body into the current body, rebinding the candidate's self-occurrences to the
// family handle so the cycle stays closed on one node. It mutates the handle in
// place under its stable identity and returns the handle. join is the value-domain
// body lattice join supplied by the producer; it must be monotone and
// finite-height so the body converges.
//
// The first widen seals the handle; later widens monotonically refine the same
// slot. Identity never changes, so the handle is Equal to itself across every
// refinement and the inter-procedural fixpoint detects a fixed point on the family
// while the body slot still settles.
//
// Ownership guard: Widen mutates only a family this interner minted. A shared or
// foreign recursive node is never SetBody'd; it is returned unchanged so an
// immutable input (stdlib/manifest/DB/cache) cannot be corrupted by one
// compilation's convergence seed.
func (i *RecursiveFamilyInterner) Widen(family *typ.Recursive, candidateBody typ.Type, join func(existing, candidate typ.Type) typ.Type) *typ.Recursive {
	if family == nil || candidateBody == nil {
		return family
	}
	if !i.owns(family) {
		return family
	}
	candidateBody = i.rebindRecursiveSelf(candidateBody, family)
	if family.Body == nil {
		family.SetBody(candidateBody)
		return family
	}
	if join == nil {
		return family
	}
	widened := join(family.Body, candidateBody)
	if widened == nil || identity.SameNode(widened, family.Body) {
		return family
	}
	family.SetBody(i.rebindRecursiveSelf(widened, family))
	return family
}

// rebindRecursiveSelf rewrites every recursive reference inside body that this
// interner knows as the same family to family itself, so the widened body keeps
// a single recursion variable.
func (i *RecursiveFamilyInterner) rebindRecursiveSelf(body typ.Type, family *typ.Recursive) typ.Type {
	if body == nil || family == nil {
		return body
	}
	return typ.Rewrite(body, func(node typ.Type) (typ.Type, bool) {
		rec, ok := node.(*typ.Recursive)
		if !ok || rec == family {
			return nil, false
		}
		if i.SameFamily(rec, family) {
			return family, true
		}
		return nil, false
	})
}
