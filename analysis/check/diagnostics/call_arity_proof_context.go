package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type callArityPresentation struct {
	Code     diagnostic.Code
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) DirectCallArity(item judgment.Judgment, primary diagnostic.Span) (callArityPresentation, bool) {
	proof := item.CallArityProof()
	if !proof.Found {
		return callArityPresentation{}, false
	}
	detail := proof.Detail
	name := item.Subject.Label
	if name == "" {
		name = "call target"
	}
	code := CodeDirectCallTooFewArgs
	labels := []diagnostic.Label{sourceLabel(primary, labelCallExpression)}
	if detail.Kind == judgment.EvidenceDetailArityTooMany {
		code = CodeDirectCallTooManyArgs
		extra := directCallArityExtraSpan(item)
		if extra.StartLine != 0 {
			labels = []diagnostic.Label{sourceLabel(extra, labelExtraArgument)}
		}
	}
	return callArityPresentation{
		Code:    code,
		Message: callArityMismatchMessage(name, detail.ExpectedCount, detail.ActualCount),
		Help:    callArityHelp(detail.ExpectedCount, detail.ActualCount),
		Labels:  labels,
		Evidence: []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: callArgumentCountEvidence(name, detail.ActualCount),
			},
			{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
				Span:    directCallArityEvidenceSpan(item, judgment.EvidenceUserAssertion),
				Message: callParameterCountEvidence(name, detail.ExpectedCount),
			},
			{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustRefuted),
				Span:    directCallArityMissingProofSpan(item),
				Message: directCallArityMissingProofMessage(detail),
			},
		},
	}, true
}

func directCallArityEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.Span {
	for _, evidence := range item.EvidenceOfKind(kind) {
		if evidence.Span.StartLine != 0 {
			return diagnosticSpanFromJudgment(evidence.Span)
		}
	}
	return diagnostic.Span{}
}

func directCallArityExtraSpan(item judgment.Judgment) diagnostic.Span {
	if len(item.Spans) < 2 {
		return diagnostic.Span{}
	}
	return diagnosticSpanFromJudgment(item.Spans[1])
}

func directCallArityMissingProofSpan(item judgment.Judgment) diagnostic.Span {
	if span := directCallArityEvidenceSpan(item, judgment.EvidenceMissingProof); span.StartLine != 0 {
		return span
	}
	return diagnosticSpanFromJudgment(item.Spans[0])
}

func directCallArityMissingProofMessage(detail judgment.EvidenceDetail) string {
	switch detail.Kind {
	case judgment.EvidenceDetailArityTooFew:
		return fmt.Sprintf("missing %d required argument%s", detail.ExpectedCount-detail.ActualCount, pluralSuffix(detail.ExpectedCount-detail.ActualCount))
	case judgment.EvidenceDetailArityTooMany:
		return fmt.Sprintf("%d extra argument%s cannot be accepted", detail.ActualCount-detail.ExpectedCount, pluralSuffix(detail.ActualCount-detail.ExpectedCount))
	default:
		return "argument count does not satisfy the callable contract"
	}
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
