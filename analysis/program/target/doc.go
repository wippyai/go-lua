// Package target holds the canonical target program: the sealed, content-addressed
// contract that every downstream consumer reads instead of the authoring specs.
//
// # Packages
//
// The closed vocabulary is its own package; the contract is this one.
//
//	target/vocabulary  the declared enums, handles and authoring specs
//	target             the sealed contract: rows, invariants, readers, identity, seal
//
// The direction is one-way and compiler-enforced: target names vocabulary, and
// vocabulary names nothing under target. vocabulary_law_test.go states it, and a
// violation is also an import cycle, so it cannot regress quietly.
//
// # Ownership and value handoffs
//
// Contract remains the root composition value for the portions of Target that
// still share its operation-column ABI. A child package is introduced only when
// its complete authority can cross the boundary as an immutable value. The
// protocol, exact-key, and boot children are immutable value cuts:
//
//	target/vocabulary  declarations and neutral handles
//	target/protocol    protocol freeze/validation, private rows, queries,
//	                   canonical identity encoding, and protocol counts
//	target/exactkey    contract-wide canonical literal directory and handles
//	target/boot       boot roots/shapes/values/entries/bindings/metatables,
//	                   private rows, queries, identity encoding and counts
//	target/operation operation geometry plus the immutable operation query plane
//	                   (Values, outcomes, behavior, transfers, effects, and
//	                   declarations); composes no Target Contract or callback
//	target             remaining verticals; composes operation.Core,
//	                   protocol.Table, exactkey.Table, boot.Table, identity and
//	                   complete Target ID
//
// `protocol.Compile` consumes sealed operation geometry and owner-issued
// callback coordinates. It accepts no Contract, operation draft, mutable
// builder, binder, callback, or receipt. The returned Table has private storage
// and no mutating public method. Target root publishes the Table directly; it
// does not wrap or re-derive its rows.
//
// The operation query cut crosses the boundary as operation.QueryInput. Seal
// drops the root construction columns after CompileQuery returns; downstream
// operation queries therefore read the Core-owned immutable value rather than
// a root walk or callback. The remaining relation columns are still root-owned
// until they receive the same complete value handoff.
//
// # Why identity sits above read
//
// Identity digests the model THROUGH the published read surface, not around it. A
// digest taken directly off private column layout would identify a storage
// arrangement; taken off the readers it identifies exactly what consumers can
// observe. Two contracts therefore share a content ID when, and only when, every
// query answers alike - which is what makes the sealed ID usable as a cache key,
// an admission key and an equivalence witness. It is also why identity may name the
// read surface but read may never name identity: a reader that consulted a digest
// would make the digest a function of itself.
package target
