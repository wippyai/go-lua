package judgment

// CallCalleeProofSummary is the renderer-facing classification of call-target
// evidence. It keeps callable/nil/member-missing selection in the judgment
// layer.
type CallCalleeProofSummary struct {
	Detail EvidenceDetail
	Found  bool
}

// CallCalleeProof returns the call-target failure detail carried by a call
// callee judgment, if present.
func (j Judgment) CallCalleeProof() CallCalleeProofSummary {
	for _, detail := range []EvidenceDetailKind{
		EvidenceDetailCalleeNotCallable,
		EvidenceDetailCalleeMayBeNil,
		EvidenceDetailMemberMissing,
	} {
		if evidence, ok := j.FirstEvidenceKindDetail(EvidenceMissingProof, detail); ok {
			return CallCalleeProofSummary{Detail: evidence.Detail, Found: true}
		}
	}
	return CallCalleeProofSummary{}
}
