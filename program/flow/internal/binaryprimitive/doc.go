// Package binaryprimitive owns Flow's narrow primitive-binary projection.
//
// The projection contains only executable arithmetic, bitwise, equality, and
// order Binary rows.  It is a derived view over the already sealed Source,
// authored Flow, candidate, and causal authorities; it does not retain any
// of those owners or introduce a second semantic vocabulary.
package binaryprimitive
