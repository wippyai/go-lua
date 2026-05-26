// Package identityrecursion is the Identity/Recursion axis of the reduced-product
// abstract value.
//
// # Axis law
//
// The Identity/Recursion axis abstracts a value's recursive product-family
// membership. Its lattice is Bottom < family < Top: Bottom is unreachable, a
// family element carries one recursive product family identified by interned
// family identity (typ.ProductFamilyHash) and compared coinductively
// (typ.SameProductFamily), and Top is the shared/unknown identity carried by
// every non-recursive value. Same-family joins stay in the family; distinct
// families have only Top above them, so their join is the family upper bound.
//
// The axis is independently sound and satisfies the recursion_via_families law:
// Equal and Hash never structurally unfold a cycle (the family hash and the
// family comparison are both coinductive), and a self-embedding tower (T, {T},
// {{T}}, ...) of one family folds to that single family element rather than
// growing without bound.
//
// The structural content of a recursive product lives on the Shape/Value axis,
// which delegates its join and coverage to the proven value-domain coinductive
// merge; this axis carries only the family identity token that keeps distinct
// recursive families distinct in the reduced product.
package identityrecursion
