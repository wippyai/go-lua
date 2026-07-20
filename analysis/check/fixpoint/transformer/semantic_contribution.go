package transformer

// semanticContribution is immutable syntax attached to one structural route.
// Its descriptor-owned recursive diagnostic fields are materialized into the
// same guarded {State, DiagnosticOutput} tuple during execution; the remaining
// non-recursive publication fields are read once from the stabilized route.
func (c semanticContribution) clone() semanticContribution {
	c.protectedCallTypestate = c.protectedCallTypestate.Clone()
	c.operations = append([]Operation(nil), c.operations...)
	c.proofs = append([]BranchProofTerm(nil), c.proofs...)
	c.refinements = append([]PathRefinementTerm(nil), c.refinements...)
	c.observations = append([]ObservationTerm(nil), c.observations...)
	c.observationObligations = append([]observationObligation(nil), c.observationObligations...)
	c.preserved = c.preserved.clone()
	c.returnConditions = append([]returnConditionParamRefinementTerm(nil), c.returnConditions...)
	c.branchLiteralCases = cloneBranchSufficientOutcomeTerms(c.branchLiteralCases)
	c.resultPublication = c.resultPublication.Clone()
	c.covariant = c.covariant.Clone()
	c.paramObligations = cloneBoundaryParamObligations(c.paramObligations)
	c.pathObligations = cloneBoundaryPathObligations(c.pathObligations)
	c.paramExposures = cloneBoundaryParamExposures(c.paramExposures)
	c.returnTransaction = c.returnTransaction.clone()
	return c
}

func (c semanticContribution) empty() bool {
	return !c.suspensionKnown && !c.maySuspend && c.protectedCallTypestate.Empty() &&
		len(c.operations) == 0 && len(c.proofs) == 0 && len(c.refinements) == 0 &&
		len(c.observations) == 0 && len(c.observationObligations) == 0 && !c.preserved.tracked && len(c.returnConditions) == 0 &&
		len(c.branchLiteralCases) == 0 && c.resultPublication.Len() == 0 && len(c.paramObligations) == 0 &&
		len(c.pathObligations) == 0 && len(c.paramExposures) == 0
}
