package judgment

// CallArgumentProofSummary is the renderer-facing classification of call
// argument proof evidence. It centralizes the mapping from low-level evidence
// details to argument proof categories.
type CallArgumentProofSummary struct {
	MayBeNil              bool
	PrecisionBoundary     bool
	GenericConflict       bool
	GenericParam          string
	GenericFunction       string
	CallParamObligation   bool
	CallParamSubjectLabel string
	CallParamDetail       EvidenceDetail
}

// Renderable reports whether a call-argument judgment should produce a
// diagnostic under the direct-call renderer.
func (s CallArgumentProofSummary) Renderable(verdict Verdict) bool {
	return verdict == VerdictRefuted || s.PrecisionBoundary
}

// CallArgumentProof returns the structured proof categories carried by a call
// argument judgment.
func (j Judgment) CallArgumentProof() CallArgumentProofSummary {
	out := CallArgumentProofSummary{
		MayBeNil:          j.HasEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailMayBeNil),
		PrecisionBoundary: j.HasEvidence(EvidencePrecisionBoundary),
	}
	if evidence, ok := j.FirstEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailGenericConflict); ok {
		out.GenericConflict = true
		out.GenericParam = evidence.Detail.Param
		out.GenericFunction = evidence.Detail.FunctionName
	}
	if evidence, ok := j.FirstEvidenceKindDetail(EvidenceUserAssertion, EvidenceDetailCallParamObligation); ok {
		out.CallParamObligation = true
		out.CallParamDetail = evidence.Detail
		out.CallParamSubjectLabel = evidence.Detail.SubjectLabel
	}
	return out
}
