// Package interproc owns the interprocedural facts product domain.
//
// It canonicalizes, joins, widens, and compares api.Facts bundles. Lower-level
// domains own individual slots: functionfact for one FunctionFact,
// returnsummary for return vectors, paramevidence for parameter evidence, and
// value for structural value relations. This package owns the product-level
// shape across graph facts, captured types, captured field writes, captured
// container mutations, constructor fields, and literal signatures.
package interproc
