package transformer

func relationCodeStepHasCoordinate(code *relationCode, step boundaryStep) bool {
	if step.kind == boundaryStepLoopFeedback || step.kind == boundaryStepLoopExit {
		return false
	}
	if step.kind != boundaryStepContribution {
		return true
	}
	return code != nil && step.contribution != 0 && int(step.contribution) < len(code.contributions) &&
		semanticContributionCarriesRecursiveDiagnostics(code.contributions[step.contribution])
}
