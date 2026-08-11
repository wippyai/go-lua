// Package semanticsource owns the closed, typed catalog boundary between
// canonical semantic sources and analysis consumers.
//
// It is deliberately cold infrastructure. It does not know Program, Target,
// Link, a domain, a Rule, or an engine. The reviewed catalog in this package
// names the complete source universe using numeric identities; publishers then
// admit one cardinality claim per declared relation exactly once.
//
// Typed rows stay in their owning source package. Program must be able to
// import this child package, so this package cannot import Program row types
// to bind or validate them without a cycle. Each generated owner wrapper is
// responsible for sealing its own typed rows before reporting a Publication.
// Publisher checks only the identity and denominator completeness of admitted
// cardinality claims; it does not establish their row derivation.
//
// Owner wrapper laws must establish that a reported count equals frozen typed rows.
// Final assembly must derive and verify publications from sealed Program,
// Target, and Link owners rather than accepting arbitrary caller reports.
package semanticsource
