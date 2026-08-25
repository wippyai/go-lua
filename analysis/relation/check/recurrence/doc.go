// Package recurrence independently checks the dependency projection of a
// sealed logical execution schema.
//
// The checker is deliberately a graph checker, not a scheduler. It consumes
// the shared check/registry view, derives each dependency's relation reads and
// writes from its canonical expression registry, derives the producer-to-consumer graph, and compares both
// projections with the declarations retained in schema/plan.  It then
// validates the declared positive SCC partition and recurrence policy.
// Monotonicity of semantic operations belongs to the binding/certificate
// layer; recurrence does not inspect or reconstruct semantic contracts.  A
// physical weak-topological order is intentionally not part of this package;
// mount specializes one from the proved SCC graph.
package recurrence
