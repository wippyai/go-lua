package judgment

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
