// Package effectrows is the EffectRows axis of the reduced-product abstract value.
//
// # Axis law
//
// The EffectRows axis abstracts the row-polymorphic effects a value's producer
// may perform (throw, io, diverge, mutate, iterator, return-derivation, ownership
// transfer, and the semantic effects). Its lattice orders rows by effect
// containment: Bottom is the pure empty row, Top is the unknown row {?}, and Join
// is the row union. A value covers another when its effect set is a superset.
//
// The axis is independently sound: Join over-approximates the combined effects of
// both producers, and Top assumes any effect (gradual). It builds on the proven
// effect.Row set operations rather than reimplementing them.
//
// # Status
//
// Phase 2 establishes the package and the local lattice surface so the Phase 5
// composition facade can import it. The lattice internals (and the reduction with
// Ownership/Escape for legality) are implemented in Phase 5; the law tests are
// skipped until then.
package effectrows
