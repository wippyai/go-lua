package obligation

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/typestate"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Judgment is the sealed state this rule's fold is issued by: the Value schema
// that owns the actuals it answers for, the Call algebra that classifies the
// operation a site dispatches to, and the reading of the protocol declarations
// those operations are bound by.
//
// It is built once per link. The declarations it reads are immutable for the
// life of the binding, so an invocation consults them and never re-derives an
// edge.
type Judgment struct {
	values *valuedomain.Schema
	calls  *calldomain.Algebra
	sealed authority
}

// Derive seals the judgment against the schemas its answer rests on.
//
// The protocol authority is reached from the Call algebra's own target
// contract rather than opened here: the contract is what the link sealed the
// declarations into, and a second reading of the same declarations would be a
// second authority over which edges exist. Pack answers where each declared
// input lands in a call row, which is the one thing that binds an operation's
// formal coordinate to the actual this rule is indexed by.
func Derive(values *valuedomain.Schema, calls *calldomain.Algebra, packs *packdomain.Schema) (Judgment, bool) {
	if values == nil || !values.Valid() || calls == nil || !calls.Valid() || packs == nil {
		return Judgment{}, false
	}
	contract, contractOK := calls.TargetContract()
	if !contractOK || contract == nil {
		return Judgment{}, false
	}
	protocols := contract.Protocols()
	sealed, sealedOK := sealAuthority(&protocols, func(operation vocabulary.Operation, input vocabulary.InputSource) (int, bool) {
		selector, selectorOK := packs.InputSelector(operation, input)
		if !selectorOK || !packs.OwnsInputSelector(selector) {
			return 0, false
		}
		return selector.Start()
	})
	if !sealedOK {
		return Judgment{}, false
	}
	return Judgment{values: values, calls: calls, sealed: sealed}, true
}

// Valid reports whether this state was sealed by Derive.
func (judgment Judgment) Valid() bool {
	return judgment.values != nil && judgment.values.Valid() &&
		judgment.calls != nil && judgment.calls.Valid() && judgment.sealed.definitions != nil
}

// Judge is the one irreducible judgment of the typestate rule: the successor
// state of the resource cell this call actual names.
//
// The verdict the same decision draws is carried by decide beside the
// successor. Both come out of one traversal of the declared edges, because a
// verdict and the move it judges are one answer about one call and a second
// pass over the same declaration would be a second authority over it.
func (judgment Judgment) Judge(
	candidate valuedomain.MountedCallArgument,
	argument valuedomain.Value,
	dispatched calldomain.Value,
	tag uint64,
	current typestate.Abstract,
) (typestate.Abstract, structure.ReductionOutcome) {
	successor, _, outcome := judgment.decide(candidate, argument, dispatched, tag, current)
	return successor, outcome
}

// decide is the whole judgment: which operation the site reaches, what that
// operation declares about this actual, and what both make of the state the
// cell is solved in.
//
// A callee the analysis cannot follow is judged rather than dropped. The call
// fact reaches here as authenticated opaque evidence, the declared escape is
// applied to the state - every proof about the resource is discharged - and
// the verdict is the unproven one. Refusing the row instead would report the
// call clean, which is the one answer a soundness judgment may not give.
func (judgment Judgment) decide(
	candidate valuedomain.MountedCallArgument,
	argument valuedomain.Value,
	dispatched calldomain.Value,
	tag uint64,
	current typestate.Abstract,
) (typestate.Abstract, typestate.Verdict, structure.ReductionOutcome) {
	if !judgment.Valid() || !judgment.values.OwnsMountedCallArgument(candidate) {
		return typestate.Unreachable(), typestate.VerdictInvalid, structure.Refuse
	}
	actual, actualOK := candidate.ActualIndex()
	if !actualOK {
		return typestate.Unreachable(), typestate.VerdictInvalid, structure.Refuse
	}
	protocol, protocolOK := protocolForTag(tag)
	if !protocolOK {
		return typestate.Unreachable(), typestate.VerdictInvalid, structure.Refuse
	}
	definition, definitionOK := judgment.sealed.definitionFor(protocol)
	if !definitionOK {
		return typestate.Unreachable(), typestate.VerdictInvalid, structure.Refuse
	}
	// A resource this call was not handed is not this cell's subject. The
	// actual's own Value fact is what says which resource the call received,
	// and an empty one names none.
	if argument.IsBottom() || dispatched.IsEmpty() {
		return current, typestate.VerdictAbstain, structure.NoCandidate
	}
	if dispatched.HasOpaqueAlternative() {
		return definition.Escape(current), unproven(current), structure.AuthenticatedOpaque
	}
	if dispatched.KnownTargetCount() == 0 {
		return current, typestate.VerdictAbstain, structure.NoCandidate
	}
	successor := typestate.Unreachable()
	// The verdict starts at no answer at all rather than at an abstention: an
	// abstention is one alternative's answer and dominates the alternatives
	// that conform, so seeding with one would report every call as withheld.
	verdict := typestate.VerdictInvalid
	for index := 0; index < dispatched.KnownTargetCount(); index++ {
		target, targetOK := dispatched.KnownTargetAt(index)
		if !targetOK {
			return typestate.Unreachable(), typestate.VerdictInvalid, structure.Refuse
		}
		reached, drawn, reachedOK := judgment.judgeTarget(definition, protocol, target, actual, current)
		if !reachedOK {
			return typestate.Unreachable(), typestate.VerdictInvalid, structure.Refuse
		}
		successor = successor.Join(reached)
		verdict = dominant(verdict, drawn)
	}
	if successor.IsUnknown() {
		return successor, verdict, structure.AuthenticatedOpaque
	}
	return successor, verdict, structure.Concrete
}

// judgeTarget decides one alternative the call site dispatches to.
//
// A target with no declared operation is a function body this analysis
// follows, so it makes no declaration about the resource and moves nothing. A
// target whose operation declares nothing about this actual is the ordinary
// case of an argument no protocol speaks about, and it is the same answer.
func (judgment Judgment) judgeTarget(
	definition typestate.Definition,
	protocol vocabulary.Protocol,
	target calldomain.Target,
	actual uint32,
	current typestate.Abstract,
) (typestate.Abstract, typestate.Verdict, bool) {
	operation, kind := judgment.calls.ClassifyTargetOperation(target)
	switch kind {
	case calldomain.TargetOperationNone:
		return current, typestate.VerdictAbstain, true
	case calldomain.TargetOperationPresent:
	default:
		return typestate.Unreachable(), typestate.VerdictInvalid, false
	}
	declared, declaredOK := judgment.sealed.obligationAt(protocol, operation, actual)
	if !declaredOK {
		return current, typestate.VerdictAbstain, true
	}
	switch declared.kind {
	case obligationNone:
		return typestate.Unreachable(), typestate.VerdictInvalid, false
	case obligationRequirement:
		return current, typestate.JudgeRequirement(current, declared.observed), true
	case obligationTransition:
		moved := typestate.Unreachable()
		for _, arrival := range declared.arrivals {
			moved = moved.Join(definition.Step(current, declared.observed, arrival))
		}
		return moved, typestate.JudgeTransition(current, declared.observed), true
	case obligationEscape:
		return definition.Escape(current), typestate.VerdictAbstain, true
	default:
		return typestate.Unreachable(), typestate.VerdictInvalid, false
	}
}

// protocolForTag reads the protocol handle out of the tag the selection
// stamped the cell row with. A selection reserves zero for "no member", so the
// handle is the tag itself: the protocol directory is one-based for the same
// reason.
func protocolForTag(tag uint64) (vocabulary.Protocol, bool) {
	if tag == 0 || tag > uint64(^uint32(0)) {
		return 0, false
	}
	return vocabulary.Protocol(tag), true
}

// unproven is the verdict an unfollowable callee draws. Which of the two
// unproven answers it is follows the question that was open: a state that is
// known says the move was not proven, and a state that is already top says the
// requirement was not proven, because there is no declared edge to speak of.
func unproven(current typestate.Abstract) typestate.Verdict {
	if current.IsUnknown() {
		return typestate.VerdictUnprovenRequirement
	}
	return typestate.VerdictUnprovenTransition
}

// dominant is the verdict of a call that reaches several alternatives: a
// finding survives an alternative that draws none, because the call may take
// the alternative that reports. Among findings the first declared one wins, so
// the answer is a function of the declaration order rather than of the order
// the dispatch happened to enumerate.
func dominant(held, drawn typestate.Verdict) typestate.Verdict {
	if !drawn.Available() {
		return held
	}
	if !held.Available() {
		return drawn
	}
	if held.Reports() != drawn.Reports() {
		if held.Reports() {
			return held
		}
		return drawn
	}
	if drawn.Ordinal() < held.Ordinal() {
		return drawn
	}
	return held
}
