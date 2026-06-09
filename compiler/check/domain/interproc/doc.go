// Package interproc owns the postflow projection product domain.
//
// It canonicalizes, joins, widens, and compares ProjectionProduct bundles for
// noncanonical compatibility/export paths. Lower-level domains own individual
// slots: functionfact for one FunctionFact projection,
// returnsummary for return vectors, paramevidence for parameter evidence, and
// value for structural value relations. This package owns the product-level
// shape across graph facts, captured types, captured field writes, captured
// container mutations, constructor fields, and literal signatures.
package interproc
