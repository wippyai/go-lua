// Package frozen holds the equivalence receipts between the parser census and
// the frozen parser-construction evidence in the grammarproof tree. Both derive
// the same two facts from the same cold sources - which whole-constructor field
// vectors a yacc action produces, and which carrier states no action can
// produce - so stating their agreement turns two derivations into one
// derivation read twice.
//
// The receipts live beside the census rather than inside it because the frozen
// evidence reaches the whole program proof graph, and the census is the
// parser-side denominator: a census that could not be built and tested without
// that graph would have taken on a dependency its own rows do not need.
package frozen
