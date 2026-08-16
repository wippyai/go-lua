// Package binding seals Flow's early lexical Cell definition roles.
//
// Binding consumes only the live Source preimage, authored Flow view, and the
// already-sealed Body parent/activation result. It owns the definition-host
// relation for Cells; it does not own Body containment or any later lexical,
// control, value, or analysis projection.
package binding
