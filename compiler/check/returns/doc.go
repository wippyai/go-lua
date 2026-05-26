// Package returns contains source-level function signature helpers.
//
// It does not run a separate return-inference loop. Return summaries, narrows,
// parameter evidence, effects, captures, and function signatures converge through
// the normal abstract-interpreter -> FunctionFact product pipeline.
//
// The helpers here answer source-shape questions needed by that pipeline:
//   - build a seed function signature from an AST function and scope;
//   - decide whether a source parameter may receive call evidence;
//   - extract finite literal/table argument shapes for evidence admission.
//
// # Signature Inference
//
// Signature helpers read source annotations and implicit-self binding metadata.
// They do not publish facts or decide convergence.
package returns
