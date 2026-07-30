package judgment

// NumericForProofSummary is the renderer-facing classification of numeric-for
// operand evidence.
type NumericForProofSummary struct {
	UserAssertion     bool
	PrecisionBoundary bool
	MissingProof      bool
}

// NumericForProof returns the proof categories carried by a numeric-for
// operand judgment.
func (j Judgment) NumericForProof() NumericForProofSummary {
	return NumericForProofSummary{
		UserAssertion:     j.HasEvidence(EvidenceUserAssertion),
		PrecisionBoundary: j.HasEvidence(EvidencePrecisionBoundary),
		MissingProof:      j.HasEvidence(EvidenceMissingProof),
	}
}
