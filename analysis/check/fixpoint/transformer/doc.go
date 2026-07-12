// Package transformer contains the inactive executable core used to compile a
// lexical function into a reusable guarded relation.
//
// The package deliberately does not participate in program solving yet.  Its
// publication boundary is the existing summary.Summary type: specialization
// is transactional and either returns one complete summary or requests the
// existing contextual solver.  This keeps the concrete solver as the oracle
// while transformer coverage is grown operation by operation and lane by lane.
package transformer
