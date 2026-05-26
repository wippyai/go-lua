// Package product defines AbstractValue, the opaque, deeply immutable, interned
// reduced product over the value-domain axes.
//
// # Layering
//
// product is the top of the value-domain stack: value <- axis <- product. Package
// value holds the low-level structural relations over typ.Type and imports no
// axis. Each axis (value/axis/*) imports value to reuse the proven structural
// logic. product imports value and every axis and composes them. The direction is
// acyclic, so the whole repository builds with no import cycle.
//
// # AbstractValue
//
// AbstractValue is the single facade of the value domain. It is a comparable
// handle to a canonical interned node holding exactly one value per axis:
// Shape/Value, Presence/Nilability, Numeric/Interval, EffectRows, Ownership,
// Escape, IdentityRecursion, and SemanticEvidence. The three foundational axes
// (Shape/Value, Presence, Numeric) carry their real lattices; the remaining axes
// carry their identity (Top) value until their phases land. Carriers elsewhere in
// the type system hold this opaque handle; raw typ.Type is recovered only through
// Project at the named diagnostic/subtype boundary.
//
// # Laws
//
//   - carrier_opacity: typ.Type leaves the domain only via Project. No other
//     accessor exposes a raw type; constructors admit typ.Type only at the
//     admission boundary (New, FromType).
//   - single_value_facade: AbstractValue is the one composed surface. Each axis is
//     its own package with a local lattice and law tests; product composes them.
//   - salsa_value_identity: AbstractValue is deeply immutable and interned, so
//     equal values share one canonical node and compare by pointer identity on the
//     fast path. It carries zero dependency, revision, or memo state. Provenance is
//     a diagnostic sidecar on the handle, excluded from Equal and Hash. Equal is
//     the total, cycle-safe lattice equivalence used as the db red-green firewall:
//     Equal(a, b) implies a.Hash() == b.Hash().
//   - recursion_via_families: recursive shape content folds via the axes'
//     coinductive product-family identity and hashing; Equal and Hash never
//     structurally unfold cycles.
//
// # Reduced product
//
// Join, Widen, Equal, Hash, and Covers are the component-wise product of the
// per-axis lattices. Construction then runs the cross-axis reduction operator of
// the reduced product, which propagates information between axes (e.g. Presence
// <-> Shape, or the Numeric index-presence bound). Reduction is a registry of
// reducers (reduce.go) driven to a local fixed point: each reducer only refines
// downward (the result is covered by its input on every axis), so reduction is
// monotone and idempotent, and a reduced value still interns canonically. The
// driver runs inside New, Join, and Widen, so callers never invoke it directly.
//
// Live reducers refine the foundational axes: presence<->shape reconciliation and
// the numeric index-presence relation. Reducers over the Ownership, Escape, and
// SemanticEvidence axes are registered as documented placeholders (identity until
// those carriers are concrete in P7), so the reduction surface is complete with no
// silent gaps.
package product
