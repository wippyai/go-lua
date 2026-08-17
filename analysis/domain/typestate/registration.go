// Package typestate owns the protocol vocabulary a module boundary declares
// finite state machines in: the protocol and state names, the FSM definition
// and its well-formedness, and the obligation set a lifecycle label is
// discharged against.
//
// All of it is declared data. A definition is authored in a manifest, decoded
// once at the module boundary, and read there; nothing in this package is an
// analyzer fact, holds solver state, or runs during a fixpoint. The
// protocol-state authority that once lived here was Link-backed and is cut,
// and this file does not restate it.
//
// # Declaration-table registration
//
// This domain declares no row on any surface of the analyzer declaration
// table, and the reason is uniform: it owns nothing any of them declares. Every
// surface can spell whatever a domain owns - a coordinate space, a rule, a
// semantic role, a publication family, a published code - so a "none" below is
// always an absence of subject matter and never a surface that has no room for
// it. The statement is per surface, because a surface with nothing to declare is
// a surface this domain says nothing about rather than one it has not reached
// yet:
//
//   - Axis: none. An axis is a coordinate space the solver writes during a
//     fixpoint and a Link instantiates a carrier for. The retained vocabulary
//     is module-boundary data with no coordinate and no carrier.
//   - Rule: none. A rule writes an axis; this domain owns none to write.
//   - Diagnostic: none. The typestate codes the fixture corpus expects have no
//     producer in this tree. The surface can spell them: a publication family is
//     a declared row of the structural vocabulary, so a code published under a
//     family the analyzer has never published before is one more row a domain
//     declares rather than a member of a catalog it would have to widen. What is
//     missing is the finding, not the spelling, and a row with no producer
//     publishes nothing.
//   - Composite: none. A composite relates declared axes.
//   - Denominator: none. A denominator names the surface entry whose universe
//     it quantifies over, and this domain owns no entry to be named.
//   - Query: none. A query family reads declared axes and publishes a result
//     codec; this domain publishes no solver-visible result.
//   - Structure: none. The structural vocabulary hosts every closed
//     process-global catalog of the analyzer: the arms, events, and outcomes,
//     the compiled occurrence families and the issuance forms that subscribe to
//     them, the publication families and observation populations, and the
//     semantic roles every other surface binds under. A domain contributes a row
//     here by owning something one of those categories names, and this domain
//     owns no coordinate space, no rule, and no published finding, so it names no
//     role and declares no member. A protocol's states are authored per module,
//     so they are not a process-global catalog with dense ordinals either.
//   - Library: none. A library contract kind addresses exported values. An FSM
//     definition attaches to no exported value, and contract instances are not
//     this surface's to hold in any case.
//
// The zero-row statement is executable: because every surface is reached by
// importing its package, a domain that declares nothing imports nothing, and
// the registration law test states exactly that. A row added here therefore
// cannot be added without this file being rewritten in the same change.
//
// # Consumers
//
// The FSM definition and the obligation vocabulary are consumed at the module
// boundary, where a manifest is decoded and its lifecycle labels are checked
// against the protocols the same manifest declares, and by the lifecycle effect
// labels that carry a protocol and a state as their payload. Neither consumer
// is a solver: both read declared data.
package typestate
