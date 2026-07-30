package transformer

import "fmt"

func relationBoundaryPrefixStep(code *relationCode, step boundaryStep) (boundaryPrefixStep, error) {
	out := boundaryPrefixStep{point: step.point, effect: step.effect, slot: step.slot, value: step.value, rootAssignment: step.rootAssignment,
		access: cloneValueAccessTerms(step.access), operands: step.operands.clone(), writes: append([]ValueTerm(nil), step.writes...), memberCall: step.memberCall, branch: step.branch.Clone(), result: step.result.Clone(), resultPhase: step.resultPhase, presence: step.presence.Clone(), channel: step.channel.Clone(), covariant: step.covariant.Clone()}
	switch step.kind {
	case boundaryStepEffect:
		out.kind = boundaryPrefixEffect
	case boundaryStepExternalCall:
		out.kind = boundaryPrefixExternalCall
	case boundaryStepRootAssignment:
		out.kind = boundaryPrefixRootAssignment
	case boundaryStepEnvironmentWrite:
		out.kind = boundaryPrefixWrite
	case boundaryStepGenericFor:
		out.kind = boundaryPrefixGenericFor
	case boundaryStepContribution:
		if code == nil || step.contribution == 0 || int(step.contribution) >= len(code.contributions) {
			return boundaryPrefixStep{}, fmt.Errorf("transformer: relation contribution is unowned")
		}
		out.kind, out.contribution = boundaryPrefixContribution, code.contributions[step.contribution].clone()
	case boundaryStepBranchRelations:
		out.kind = boundaryPrefixBranchRelations
	case boundaryStepCallResults:
		out.kind = boundaryPrefixCallResults
	case boundaryStepPresenceImplications:
		out.kind = boundaryPrefixPresenceImplications
	case boundaryStepChannelSelect:
		out.kind = boundaryPrefixChannelSelect
	case boundaryStepCovariantExposure:
		out.kind = boundaryPrefixCovariantExposure
	case boundaryStepLoopFeedback, boundaryStepLoopExit:
		return boundaryPrefixStep{}, fmt.Errorf("transformer: lexical loop control requires direct tuple-mu lowering")
	default:
		return boundaryPrefixStep{}, fmt.Errorf("transformer: relation step has invalid syntax")
	}
	return out, nil
}
