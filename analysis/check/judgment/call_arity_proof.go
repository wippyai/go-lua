package judgment

// CallArityProofSummary is the renderer-facing classification of call arity
// evidence. It keeps the too-few/too-many choice owned by the judgment layer.
type CallArityProofSummary struct {
	Detail EvidenceDetail
	Found  bool
}

// CallArityProof returns the arity mismatch detail carried by a call arity
// judgment, if present.
func (j Judgment) CallArityProof() CallArityProofSummary {
	if evidence, ok := j.FirstEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailArityTooFew); ok {
		return CallArityProofSummary{Detail: evidence.Detail, Found: true}
	}
	if evidence, ok := j.FirstEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailArityTooMany); ok {
		return CallArityProofSummary{Detail: evidence.Detail, Found: true}
	}
	return CallArityProofSummary{}
}
