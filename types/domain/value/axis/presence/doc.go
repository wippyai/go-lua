// Package presence is the Presence/Nilability axis of the reduced-product
// abstract value.
//
// # Axis law
//
// The Presence/Nilability axis abstracts whether a runtime slot holds a value.
// Its lattice is the four-point chain:
//
//	Bottom < {Present, Absent} < Maybe
//
// Bottom is unreachable (no concrete state). Present means the slot definitely
// holds a non-nil value; Absent means the slot is definitely nil/missing. Present
// and Absent are incomparable siblings; their join is Maybe (the slot may or may
// not hold a value). Maybe is Top.
//
// The axis is independently sound: Present over-approximates "non-nil on this
// path", Absent over-approximates "nil on this path", and Maybe over-approximates
// both. Join is the least upper bound of the chain, Widen equals Join (the
// lattice has finite height so no acceleration is needed), and Covers is the
// reflexive order. No other axis is consulted; presence<->shape reduction happens
// in the facade.
package presence
