// Package interproc owns postflow projection lane convergence.
//
// It canonicalizes, joins, widens, and compares the remaining noncanonical
// compatibility/export lanes independently: function facts, captured types,
// captured field writes, and constructor fields. Lower-level domains own the
// slot semantics; this package owns only the finite-height convergence laws for
// these projection lanes.
package interproc
