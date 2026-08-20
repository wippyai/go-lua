// Package conformance owns the type domain's assignment verdict: whether the
// families a value may carry are families the type its target declares admits.
//
// The verdict is a pure function of two may-sets over the closed Lua runtime
// vocabulary. It holds no graph, reads no coordinate space, and reaches no
// program: what a declaration admits and what a value was observed to carry are
// both decided before it is called, by the authorities that own them, and this
// package states the one relation between them. The publication half calls it;
// nothing else in the domain does.
//
// # Why sets
//
// A declaration is an upper bound on the value its target holds, and an
// observation is an upper bound on the value that reached it. Both bounds are
// exactly may-sets of runtime families at this granularity, so the judgment is
// subset containment and nothing else. An interface-bounded value keeps the
// whole vocabulary rather than losing its bound, and that is stated here as an
// abstention rather than repaired by the caller: this package consumes sets, so
// how a bound became the whole vocabulary is not its question.
package conformance

import "github.com/wippyai/go-lua/domain/runtimekind"

// Verdict is the closed catalog of answers this judgment gives.
type Verdict uint8

const (
	// VerdictInvalid is the absent answer. It is returned for a set that is not
	// a set over the closed vocabulary, so a caller cannot read a defect of its
	// own as a judgment about a program.
	VerdictInvalid Verdict = iota
	// VerdictAbstain is the answer when the observation decides nothing: no
	// value reached the target, or the value's families were never narrowed
	// below the whole vocabulary.
	VerdictAbstain
	// VerdictConforms is the answer when every family the value may carry is a
	// family the declaration admits.
	VerdictConforms
	// VerdictViolates is the answer when the value may carry a family the
	// declaration does not admit.
	VerdictViolates
	// VerdictMayBeNil is the answer when the declaration excludes nil, the
	// value may carry nil, and every family the value may carry besides nil is
	// a family the declaration admits. It names the nil-presence finding apart
	// from a general containment violation.
	VerdictMayBeNil
	// VerdictMemberAbsent is the answer for a declared-required key that is
	// not established. This package names the verdict; the key-set comparison
	// that decides it is the issuance layer's, not a judgment this package
	// computes.
	VerdictMemberAbsent
	// VerdictUnproven is the answer for a requirement no derivation in this
	// package establishes. It is reserved for a judgment owned elsewhere.
	VerdictUnproven
	verdictLimit
)

// Available reports membership in the closed verdict catalog.
func (verdict Verdict) Available() bool {
	return verdict > VerdictInvalid && verdict < verdictLimit
}

// MayKindConformance is the assignment verdict over runtime families.
// declaredMay is the set of families the target's declared type admits, and
// observed is the set of families the assigned value may carry.
//
// The judgment is containment: the value conforms exactly when it may carry no
// family outside the declaration. Two observations decide nothing and are
// answered as abstentions rather than as either side of the containment - an
// empty observation is a target no value reached, and an observation of the
// whole vocabulary is a value nothing narrowed.
//
// A declaration that admits nothing admits nothing: an observed family against
// it violates, the same as any other family outside the declared set. A caller
// with no declaration to enforce has no judgment to ask for.
func MayKindConformance(declaredMay, observed runtimekind.Set) Verdict {
	if !declaredMay.Valid() || !observed.Valid() {
		return VerdictInvalid
	}
	if observed == 0 || observed == runtimekind.All {
		return VerdictAbstain
	}
	if observed&^declaredMay != 0 {
		return VerdictViolates
	}
	return VerdictConforms
}

// MayBeNilConformance is the nil-presence verdict over runtime families. It
// answers the narrower question of whether an observation that would
// otherwise violate a declaration violates it for no reason beyond nil: the
// declaration excludes nil, the value may carry nil, and every other family
// the value may carry is one the declaration admits.
//
// This is not a restatement of MayKindConformance's containment: an
// observation that carries a family the declaration does not admit besides
// nil is answered as an abstention here, because the finding it names is a
// general violation, not a nil-presence one. The two functions are called
// together by a caller that wants to tell the two findings apart; neither
// calls the other.
func MayBeNilConformance(declaredMay, observed runtimekind.Set) Verdict {
	if !declaredMay.Valid() || !observed.Valid() {
		return VerdictInvalid
	}
	if observed == 0 || observed == runtimekind.All {
		return VerdictAbstain
	}
	if declaredMay.Contains(runtimekind.Nil) || !observed.Contains(runtimekind.Nil) {
		return VerdictAbstain
	}
	nonNil := observed &^ runtimekind.Bit(runtimekind.Nil)
	if nonNil&^declaredMay != 0 {
		return VerdictAbstain
	}
	return VerdictMayBeNil
}
