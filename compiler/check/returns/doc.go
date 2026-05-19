// Package returns orchestrates local return inference and interprocedural fact
// products.
//
// It does not own the lattice laws for individual fact slots. Those live in
// domain packages:
//   - domain/paramevidence owns parameter evidence;
//   - domain/returnsummary owns return vectors and function-return alignment;
//   - domain/functionfact owns one api.FunctionFact at a time;
//   - domain/value owns reusable structural value relations.
//
// This package owns when those domains are applied across maps, SCCs, overlays,
// captured mutations, and recursive interprocedural fixpoint boundaries.
//
// # SCC-Based Analysis
//
// Functions are grouped into strongly connected components (SCCs):
//   - Non-recursive functions: analyzed once
//   - Mutually recursive groups: iterated until types stabilize
//
// [ComputeSymbolSCCs] uses Tarjan's algorithm to find SCCs in reverse
// topological order, ensuring callees are analyzed before callers.
//
// # Return Type Computation
//
// For each function:
//   - Collect return expressions from all return statements
//   - Synthesize types for return expressions
//   - Merge candidate return vectors through domain/returnsummary
//   - Apply product-level widening for recursive convergence
//
// # Type Widening
//
// To ensure termination in recursive cases, types are widened:
//   - After N iterations, recursive types are approximated
//   - Widening preserves soundness while ensuring convergence
//
// # Overlay System
//
// [Overlay] provides the mutable return-summary layer:
//   - Stable summaries from previous iterations
//   - Pending updates from current iteration
//   - Atomic commit when iteration converges
//
// # Call Graph Construction
//
// [BuildCallGraph] extracts the local function call graph from CFGs,
// identifying which functions call which others within the module.
//
// # Signature Inference
//
// Signature inference combines parameter evidence and return types to produce
// complete function signatures for functions without annotations.
package returns
