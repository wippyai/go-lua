package judgment

// LifecycleProofSummary is the renderer-facing classification of a lifecycle
// obligation proof.
type LifecycleProofSummary struct {
	Detail EvidenceDetail
	Found  bool
}

// LifecycleProof returns the missing lifecycle obligation detail carried by a
// lifecycle judgment, if present.
func (j Judgment) LifecycleProof() LifecycleProofSummary {
	if evidence, ok := j.FirstEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailLifecycleMissingProof); ok {
		return LifecycleProofSummary{Detail: evidence.Detail, Found: true}
	}
	return LifecycleProofSummary{}
}
