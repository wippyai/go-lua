// Package arithmetic owns the small physical arithmetic parity world.
//
// The package is test data, not an execution surface.  Its declaration is a
// closed relational plan with three explicit semantic inputs.  The mount,
// state publisher, typed worker, and oracle adapter all consume that same
// declaration so a test cannot hide an input in a full-row callback.
package arithmetic
