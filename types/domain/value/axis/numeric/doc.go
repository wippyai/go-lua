// Package numeric is the Numeric/Interval axis of the reduced-product abstract value.
//
// # Axis law
//
// The Numeric/Interval axis abstracts the integer value of a runtime value as an
// interval [lower, upper] refined by an optional modular residue (x mod m == r).
// Its lattice is: Bottom is the empty interval (no integer satisfies the
// constraints), Top is the unbounded interval with no residue, and Join is the
// convex interval hull with the residue kept only when both sides agree. A value
// v1 covers v2 when the integer set described by v2 is a subset of the set
// described by v1.
//
// The axis is independently sound: each operation over-approximates the set of
// integers a runtime value may take. Join returns a superset of both operands'
// integer sets, Widen drops a moving bound to infinity (a sound accelerant that
// guarantees termination), and Covers is exactly interval-plus-residue
// containment. No other axis is consulted; reduction happens in the facade.
//
// # Reuse
//
// The axis reuses the proven interval representation (numeric.Interval, with its
// MinInt64/MaxInt64 unbounded convention) and the modular primitive
// (theory.ModularFact, with its normalized residue and Check). The existing
// numeric.State / numeric.Domain are path-keyed relational stores (a DBM over
// named variables); this axis is the value-level component for a single value,
// so it composes those primitives into a self-contained single-value lattice
// rather than reimplementing them.
package numeric
