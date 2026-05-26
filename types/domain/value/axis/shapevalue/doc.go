// Package shapevalue is the Shape/Value axis of the reduced-product abstract value.
//
// # Axis law
//
// The Shape/Value axis abstracts the structural type of a runtime value. Its
// lattice is the gradual subtype/precision order over typ.Type: Bottom is the
// uninhabited type (never), Top is the fully-dynamic type (any), and Join is the
// least upper bound under convergence widening. A value v1 covers v2 when every
// concrete value described by v2 is also described by v1.
//
// The axis is independently sound: each operation is defined purely over the
// structural type lattice and preserves the relation "the abstract value
// over-approximates the set of concrete runtime values". Join over-approximates
// the union of two value sets, Widen is a sound accelerant that never loses
// soundness, and Covers is exactly the lattice order. No cross-axis information
// is consulted here; reduction with other axes happens in the composition facade.
//
// # Boundary
//
// This package is value-domain internal: it may operate on raw typ.Type and call
// generic typ ops (the no_generic_type_ops law carves out value-domain internals).
// It reuses the proven convergence join/widen/equality from package value rather
// than reimplementing structural merge logic. typ.Type never escapes this axis
// except through Project, the named diagnostic/subtype boundary.
package shapevalue
