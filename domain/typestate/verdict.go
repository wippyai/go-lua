package typestate

// Verdict is this domain's closed judgment vocabulary: the complete set of
// answers a typestate question has. It is the judgment itself, not a published
// finding - which verdicts a diagnostic surface publishes, and under which
// code, is that surface's declaration. Keeping the two apart is what lets one
// verdict be published as `channel.send.closed` for one protocol and as
// `typestate.invalid_requirement` for another without the judgment knowing
// either spelling.
type Verdict uint8

const (
	// VerdictInvalid is the zero value and is never a judgment.
	VerdictInvalid Verdict = iota
	// VerdictAbstain is the refusal to judge: the program point is
	// unreachable, or the question does not apply there. It is not a claim
	// that the program is correct.
	VerdictAbstain
	// VerdictConforms is a proof that the usage satisfies the declaration.
	VerdictConforms
	// VerdictInvalidRequirement proves an operation that reads a resource
	// without moving it runs in a state its declaration excludes.
	VerdictInvalidRequirement
	// VerdictUnprovenRequirement states that the required state is not proven
	// and not refuted. It is the sound answer to an unknown state: the
	// analysis will not claim the requirement holds.
	VerdictUnprovenRequirement
	// VerdictInvalidTransition proves an operation that moves a resource runs
	// from a state its declared edge does not leave.
	VerdictInvalidTransition
	// VerdictUnprovenTransition states that the declared source state is
	// neither proven nor refuted at a moving operation.
	VerdictUnprovenTransition
	// VerdictUnreleasedState proves a resource ends local ownership in one
	// known state that discharges no obligation.
	VerdictUnreleasedState
	// VerdictUnreleasedNonFinal proves a resource ends local ownership with
	// several possible states, at least one of which discharges no
	// obligation. No single state can be named in the report.
	VerdictUnreleasedNonFinal
	verdictLimit
)

// Available reports whether the verdict is one of the declared judgments.
func (verdict Verdict) Available() bool {
	return verdict > VerdictInvalid && verdict < verdictLimit
}

// Ordinal is the verdict's dense position. It is the identity a declaration
// surface keys a published variant by.
func (verdict Verdict) Ordinal() uint16 {
	if !verdict.Available() {
		return 0
	}
	return uint16(verdict)
}

var verdictSpellings = [verdictLimit]string{
	VerdictAbstain:             "abstain",
	VerdictConforms:            "conforms",
	VerdictInvalidRequirement:  "invalid-requirement",
	VerdictUnprovenRequirement: "unproven-requirement",
	VerdictInvalidTransition:   "invalid-transition",
	VerdictUnprovenTransition:  "unproven-transition",
	VerdictUnreleasedState:     "unreleased-state",
	VerdictUnreleasedNonFinal:  "unreleased-non-final",
}

// Spelling is the verdict's external name.
func (verdict Verdict) Spelling() string {
	if !verdict.Available() {
		return ""
	}
	return verdictSpellings[verdict]
}

// Catalog returns every declared verdict in ordinal order.
func Catalog() []Verdict {
	out := make([]Verdict, 0, int(verdictLimit)-1)
	for verdict := VerdictAbstain; verdict < verdictLimit; verdict++ {
		out = append(out, verdict)
	}
	return out
}

// Reports whether the verdict is a finding rather than a clean or withheld
// answer. A surface publishes exactly the reporting verdicts it declares a
// code for.
func (verdict Verdict) Reports() bool {
	switch verdict {
	case VerdictInvalidRequirement, VerdictUnprovenRequirement,
		VerdictInvalidTransition, VerdictUnprovenTransition,
		VerdictUnreleasedState, VerdictUnreleasedNonFinal:
		return true
	default:
		return false
	}
}

// JudgeRequirement decides one operation that requires a resource to be in
// required and does not move it.
//
// The decision is one-directional: a violation is reported only when the
// solved state proves the required state is impossible, and everything short
// of that proof - an unknown state, or a set that keeps the required state
// among several possibilities - is unproven rather than clean. An analysis
// that cannot see the state therefore never certifies the call.
func JudgeRequirement(solved Abstract, required State) Verdict {
	if required == "" || solved.Unreachable() {
		return VerdictAbstain
	}
	switch {
	case solved.Proves(required):
		return VerdictConforms
	case solved.Refutes(required):
		return VerdictInvalidRequirement
	default:
		return VerdictUnprovenRequirement
	}
}

// JudgeTransition decides one operation whose declared edge leaves from.
//
// It is the same one-directional decision as JudgeRequirement, reported under
// the transition verdicts so a surface can publish a move against a wrong
// state separately from a read against one.
func JudgeTransition(solved Abstract, from State) Verdict {
	if from == "" || solved.Unreachable() {
		return VerdictAbstain
	}
	switch {
	case solved.Proves(from):
		return VerdictConforms
	case solved.Refutes(from):
		return VerdictInvalidTransition
	default:
		return VerdictUnprovenTransition
	}
}

// JudgeExit decides the obligation of one acquired resource where local
// ownership ends.
//
// An obligation with no discharging state is nothing to answer for. An unknown
// state is an abstention, never a leak: a resource whose identity left the
// analysis may well have been released, and claiming otherwise would be a
// finding about the analysis rather than about the program. A leak is reported
// only when every possible exit state is known and at least one of them
// discharges nothing; the two reporting verdicts differ only in whether one
// state can be named.
func JudgeExit(solved Abstract, obligation Obligation) Verdict {
	if obligation.Empty() || solved.Unreachable() || solved.IsUnknown() {
		return VerdictAbstain
	}
	states := solved.States().List()
	if len(states) == 0 {
		return VerdictAbstain
	}
	for _, state := range states {
		if obligation.SatisfiedBy(state) {
			continue
		}
		if len(states) == 1 {
			return VerdictUnreleasedState
		}
		return VerdictUnreleasedNonFinal
	}
	return VerdictConforms
}
