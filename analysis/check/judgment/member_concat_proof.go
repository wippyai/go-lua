package judgment

// MemberReadProofSummary is the renderer-facing classification of a missing
// member-read proof.
type MemberReadProofSummary struct {
	Detail EvidenceDetail
	Found  bool
}

// MemberReadProof returns the missing member detail carried by a member-read
// judgment, if present.
func (j Judgment) MemberReadProof() MemberReadProofSummary {
	if evidence, ok := j.FirstEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailMemberMissing); ok && evidence.Detail.Field != "" {
		return MemberReadProofSummary{Detail: evidence.Detail, Found: true}
	}
	return MemberReadProofSummary{}
}

// ConcatOperandProofSummary is the renderer-facing classification of a concat
// operand proof.
type ConcatOperandProofSummary struct {
	Detail EvidenceDetail
	Found  bool
}

// ConcatOperandProof returns the concat operand detail carried by a concat
// operand judgment, if present.
func (j Judgment) ConcatOperandProof() ConcatOperandProofSummary {
	if evidence, ok := j.FirstEvidenceKindDetail(EvidenceAbstractFact, EvidenceDetailConcatOperand); ok && evidence.Detail.Field != "" {
		return ConcatOperandProofSummary{Detail: evidence.Detail, Found: true}
	}
	return ConcatOperandProofSummary{}
}
