// Package transformer compiles each lexical function into typed WorldProgram
// input IR, reduces a complete call/resource SCC once into normalized
// parametric boundary relations, and applies those relations through lazy
// binding environments. The abstract domains remain owned by State; this
// package changes orchestration, not semantic axes.
//
// WorldProgram is never interpreted per call. Calls reference reduced relation
// variables, and evaluator application cells own the sole semantic fixed point
// with WTO widening/narrowing. There is no contextual fallback, depth budget,
// row budget, or recursion cutoff in this pipeline.
package transformer
