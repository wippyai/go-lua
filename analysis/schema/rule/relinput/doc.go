// Package relinput owns the relation input bundle: the sealed row table that
// states, for every dense rule ordinal in one compiled rule catalog, the
// decision scope the rule's candidate rows are decided at, the decision scope
// each declared input port observes, and the owner-issued region evidence
// every named scope stands on.
//
// The bundle is a publication of composition knowledge, not a derivation. A
// decision scope names a relation-schema conjunction; it is not a Program
// geometry identity and cannot be recovered from one. The composition that
// installs a rule is the only authority that knows which scope a candidate is
// decided at and which scope a port observes, so this package takes that
// answer from the composition and seals it against the rule catalog it was
// answered for.
//
// The table is total over the rule catalog. Every rule ordinal takes a row,
// and a rule whose scope conjunction the composition cannot resolve refuses
// the whole seal with that ordinal named. A rule is never dropped, and a
// missing scope is never replaced by a default.
package relinput
