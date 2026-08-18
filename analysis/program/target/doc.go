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
// # Why seal and query are not packages
//
// Inside this package the code is layered into four altitudes (below), and the
// natural question is why the upstream (seal) and downstream (read, identity)
// altitudes are not packages of their own the way the vocabulary is. Two reasons,
// and neither is about effort:
//
// The read surface IS Contract's method set. Go admits a method declaration only
// in the package that declares its receiver, so a query package could not carry
// `func (c *Contract) OperationCount() int` at all: every reader would have to
// become a free function over an exported *target.Contract.
//
// That in turn requires exporting the column layout. Seal writes the columns and
// the readers read them; separating writers from readers across a package boundary
// means promoting every private slice, row type and row field. The measured cost of
// the cut is 310 promoted symbols, and the result is worse than large - it is
// unsound, because "immutable after Seal" stops being enforceable once any package
// can assign to a sealed contract's columns.
//
// So seal, read and identity share one package because they address one private
// layout. What keeps them honest is the altitude law below rather than an import
// graph. Do not re-propose the split without first removing the reason for it: the
// only version that works is one where the columns are not private, and that is a
// different contract.
//
// # Why identity is not a package either
//
// Identity looks like the exception, because its whole output is a byte stream and
// a byte stream is a seam. Measured, it is not: that seam is already factored, and
// what remains is not encoding.
//
// The digest construction is internal/framing.Writer streaming into sha256, the ID
// type is analysis/identity.ContentID, and semanticID is twenty lines binding them
// to a domain tag and a codec version. A child package could own those twenty
// lines. It would gain one declaration.
//
// The other sixty-one declarations of the plane are the canonical walk: they read
// private columns to build the closure semanticID digests. Forty-nine of them are
// methods on Contract, seventeen of those the published identity API. Moving the
// walk means exporting the columns, which is the cut already ruled out above; and
// moving only the twenty lines adds a package boundary while reducing no coupling.
//
// The general form: a plane extracts when its coupling reduces to a value handoff.
// Identity's coupling does not reduce to its byte stream, because the stream is
// produced BY the coupling rather than in place of it. The vocabulary extracted
// because it hands over declarations and reads nothing back.
//
// # Altitudes
//
// References run one way only, from a higher altitude to a lower one; the core
// references nothing above it, and nothing references the seal.
//
//	core     the row model and the invariants it is closed under
//	read     the published query surface over the sealed rows
//	identity the content identity computed over that surface
//	seal     the freeze that turns authored specs into rows
//
// core - model_rows.go, model_invariants.go, model_publication.go, spec.go,
// checked.go. The row layouts, the predicates a row is required to satisfy, and the
// two sealed value types whose fields stay private (a PublicationEffectDescriptor
// cannot be forged, and a Spec is consumed once). Core names no query, no digest
// and no freeze: an invariant that has to consult a reader is not an invariant of
// the model.
//
// read - operation_query.go, invocation_query.go, continuation_query.go,
// boot_query.go, protocol_query.go, subedge_relation.go, counts.go. The accessors a
// consumer observes the contract through. They read core columns and answer in
// declared vocabularies; they never write a column and never reach into the freeze.
//
// identity - contentid*.go, semantic_identity.go, effect_identity.go,
// relation_identity.go, identity_encoding.go. The content identity of the sealed
// program, plus the identity-exclusive columns it owns.
//
// seal - seal.go, seal_drafts.go, seal_relations.go, seal_validation.go,
// seal_operation*.go, seal_append*.go, seal_resolution*.go, subedge_freeze*.go,
// subedge_relation_seal.go, subedge_rows.go, subedge_validation.go, boot.go,
// protocol.go, exact_key*.go, values_projection.go. Drafting, validation,
// canonicalisation and the append ABI that writes core columns. The seal is the
// only altitude that mutates a Contract; once it returns the contract is immutable
// and only the three altitudes below it are exercised.
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
//
// altitude_law_test.go states the one-way reference law over this file map.
package target
