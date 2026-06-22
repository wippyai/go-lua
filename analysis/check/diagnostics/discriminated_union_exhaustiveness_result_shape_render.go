package diagnostics

import "github.com/wippyai/go-lua/analysis/diagnostic"

func newResultShapeExhaustivenessDiagnostic(evidence resultShapeEvidence) diagnostic.Diagnostic {
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.readSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     resultShapeExhaustivenessMessage(evidence.readPath, evidence.requiredCase),
		Explanation: resultShapeExhaustivenessExplanation(evidence),
		Help:        resultShapeExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(evidence.readSpan, labelResultFieldRead)},
	})
}

func resultShapeExhaustivenessExplanation(evidence resultShapeEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.readSpan,
			Message: resultShapeUnionEvidence(evidence.receiver, evidence.discriminant),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.readSpan,
			Message: resultShapeFieldCaseEvidence(evidence.readPath, evidence.requiredCase),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.readSpan,
			Message: resultShapeMissingProofEvidence(evidence.requiredCase),
		},
	)
}
