// Package runtime composes one already-mounted execution, relation state,
// physical operators, semantic bindings, and the fixpoint queue. It owns the
// serial coordinator only: planning and operator selection belong to mount,
// state roots belong to state/database, and semantic payloads belong to the
// mounted axis algebra. Solve advances state solely from authenticated
// publish settlements and stops when the sealed queue is empty.
package runtime
