package judgment

// ReturnProofSummary is the renderer-facing classification of return proof
// evidence.
type ReturnProofSummary struct {
	MayBeNil          bool
	IndexedRead       bool
	PrecisionBoundary bool
}

// ReturnProof returns the proof categories carried by a return judgment.
func (j Judgment) ReturnProof() ReturnProofSummary {
	return ReturnProofSummary{
		MayBeNil:          j.ReturnMissingProofMayBeNil(),
		IndexedRead:       j.ReturnMissingProofIndexedRead(),
		PrecisionBoundary: j.HasEvidence(EvidencePrecisionBoundary),
	}
}

// ReturnMissingProofMayBeNil reports whether the missing return proof is about
// nilability, including indexed reads that can miss or read nil.
func (j Judgment) ReturnMissingProofMayBeNil() bool {
	return j.HasAnyEvidenceKindDetail(
		EvidenceMissingProof,
		EvidenceDetailMayBeNil,
		EvidenceDetailIndexedReadMissingProof,
	)
}

// ReturnMissingProofIndexedRead reports whether the return proof failed
// because an indexed read lacks a validation proof.
func (j Judgment) ReturnMissingProofIndexedRead() bool {
	return j.HasEvidenceKindDetail(EvidenceMissingProof, EvidenceDetailIndexedReadMissingProof)
}
