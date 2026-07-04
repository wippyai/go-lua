package judgment

// AssignmentProofSummary is the renderer-facing classification of assignment
// proof evidence. It centralizes the mapping from low-level evidence details to
// user-visible proof categories, so renderers do not independently interpret
// the judgment evidence chain.
type AssignmentProofSummary struct {
	MayBeNil          bool
	IndexedRead       bool
	CallResult        bool
	CallInvalidated   bool
	DynamicTarget     bool
	PrecisionBoundary bool
}

// AssignmentProofReason classifies the semantic reason an assignment proof is
// missing without importing diagnostic-layer reason codes.
type AssignmentProofReason uint8

const (
	AssignmentProofReasonUnspecified AssignmentProofReason = iota
	AssignmentProofReasonIndexedRead
	AssignmentProofReasonBoundaryValidation
)

// Reason returns the proof reason renderers should map to diagnostic evidence
// metadata.
func (s AssignmentProofSummary) Reason() AssignmentProofReason {
	if s.IndexedRead {
		return AssignmentProofReasonIndexedRead
	}
	if s.PrecisionBoundary || s.CallResult {
		return AssignmentProofReasonBoundaryValidation
	}
	return AssignmentProofReasonUnspecified
}

// BoundaryProofMissing reports whether the missing proof should be explained as
// a boundary validation failure rather than a plain declared-type proof.
func (s AssignmentProofSummary) BoundaryProofMissing() bool {
	return s.CallInvalidated || s.DynamicTarget
}

// AssignmentProof returns the structured proof categories carried by an
// assignment judgment.
func (j Judgment) AssignmentProof() AssignmentProofSummary {
	return AssignmentProofSummary{
		MayBeNil:          j.AssignmentMissingProofMayBeNil(),
		IndexedRead:       j.AssignmentMissingProofIndexedRead(),
		CallResult:        j.HasEvidenceDetail(EvidenceDetailCallResultAssignment),
		CallInvalidated:   j.AssignmentHasCallInvalidationEvidence(),
		DynamicTarget:     j.AssignmentHasDynamicTargetEvidence(),
		PrecisionBoundary: j.HasEvidence(EvidencePrecisionBoundary),
	}
}

// AssignmentCallResultDetail returns the call-result evidence attached to an
// assignment judgment.
func (j Judgment) AssignmentCallResultDetail() (EvidenceDetail, bool) {
	if evidence, ok := j.FirstEvidenceDetail(EvidenceDetailCallResultAssignment); ok {
		return evidence.Detail, true
	}
	return EvidenceDetail{}, false
}

// AssignmentUnderSuppliedCallResultDetail reports that a target receives a
// result slot the callee cannot produce.
func (j Judgment) AssignmentUnderSuppliedCallResultDetail() (EvidenceDetail, bool) {
	detail, ok := j.AssignmentCallResultDetail()
	return detail, ok && detail.UnderSupplied
}

// AssignmentCallResultReturnSpan returns the span of the declared return slot
// that justified the call-result type, when available.
func (j Judgment) AssignmentCallResultReturnSpan() (SpanRef, bool) {
	evidence, ok := j.FirstEvidenceKindDetail(EvidenceUserAssertion, EvidenceDetailCallResultAssignment)
	if ok && evidence.Span.StartLine != 0 {
		return evidence.Span, true
	}
	return SpanRef{}, false
}

// AssignmentMissingRequiredField returns the structural field missing from the
// assigned value, when the assignment judgment carries that proof.
func (j Judgment) AssignmentMissingRequiredField() (EvidenceDetail, bool) {
	if evidence, ok := j.FirstEvidenceDetail(EvidenceDetailMissingRequiredField); ok && evidence.Detail.Field != "" {
		return evidence.Detail, true
	}
	return EvidenceDetail{}, false
}

// AssignmentMissingRequiredMethod returns the interface method missing from the
// assigned value, when the assignment judgment carries that proof.
func (j Judgment) AssignmentMissingRequiredMethod() (EvidenceDetail, bool) {
	if evidence, ok := j.FirstEvidenceDetail(EvidenceDetailMissingRequiredMethod); ok && evidence.Detail.Field != "" {
		return evidence.Detail, true
	}
	return EvidenceDetail{}, false
}

// AssignmentMethodTypeMismatch returns the interface method whose type does not
// satisfy the expected method type.
func (j Judgment) AssignmentMethodTypeMismatch() (EvidenceDetail, bool) {
	if evidence, ok := j.FirstEvidenceDetail(EvidenceDetailMethodTypeMismatch); ok && evidence.Detail.Field != "" {
		return evidence.Detail, true
	}
	return EvidenceDetail{}, false
}

// AssignmentHasCallInvalidationEvidence reports whether a prior call may have
// invalidated the assignment source proof.
func (j Judgment) AssignmentHasCallInvalidationEvidence() bool {
	return j.HasEvidenceDetail(EvidenceDetailAssignmentCallInvalidation)
}

// AssignmentHasDynamicTargetEvidence reports whether the target type came from
// a dynamic assignment target proof rather than a declaration.
func (j Judgment) AssignmentHasDynamicTargetEvidence() bool {
	return j.HasEvidenceDetail(EvidenceDetailDynamicAssignmentTarget)
}

// AssignmentMissingProofMayBeNil reports whether the missing proof is about
// nilability, including indexed reads that can miss or read nil.
func (j Judgment) AssignmentMissingProofMayBeNil() bool {
	return j.HasAnyEvidenceKindDetail(
		EvidenceMissingProof,
		EvidenceDetailMayBeNil,
		EvidenceDetailIndexedReadMissingProof,
	)
}

// AssignmentMissingProofIndexedRead reports whether the assignment failed
// because an indexed read lacks a validation proof.
func (j Judgment) AssignmentMissingProofIndexedRead() bool {
	return j.HasEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailIndexedReadMissingProof)
}

// AssignmentUserAssertedAnyEvidence returns explicit any assertions relevant to
// an assignment judgment.
func (j Judgment) AssignmentUserAssertedAnyEvidence() EvidenceChain {
	return j.EvidenceKindDetails(EvidenceUserAssertion, EvidenceDetailUserAssertedAny)
}

// AssignmentNilableAccessEvidence returns abstract facts explaining a nilable
// access that contributed to the assignment judgment.
func (j Judgment) AssignmentNilableAccessEvidence() EvidenceChain {
	return j.EvidenceKindDetails(EvidenceAbstractFact, EvidenceDetailMayBeNil)
}

// AssignmentSourceContributionEvidence returns prior-assignment facts that
// contributed a later source shape to the assignment read.
func (j Judgment) AssignmentSourceContributionEvidence() EvidenceChain {
	return j.EvidenceKindDetails(EvidenceAbstractFact, EvidenceDetailAssignmentSourceContribution)
}

// AssignmentCallInvalidationEvidence returns facts describing calls that may
// have invalidated the source proof before the assignment read.
func (j Judgment) AssignmentCallInvalidationEvidence() EvidenceChain {
	return j.EvidenceKindDetails(EvidenceAbstractFact, EvidenceDetailAssignmentCallInvalidation)
}
