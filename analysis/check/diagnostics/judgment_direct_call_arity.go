package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceDirectCallArityJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.DirectCallArity{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func renderDirectCallArityJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeCallArity || item.Subject.Kind != judgment.SubjectCallExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	detail, ok := directCallArityDetail(item)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	name := item.Subject.Label
	if name == "" {
		name = "call target"
	}
	code := CodeDirectCallTooFewArgs
	labels := []diagnostic.Label{sourceLabel(diagnosticSpanFromJudgment(item.Spans[0]), labelCallExpression)}
	if detail.Kind == judgment.EvidenceDetailArityTooMany {
		code = CodeDirectCallTooManyArgs
		extra := directCallArityExtraSpan(item)
		if extra.StartLine != 0 {
			labels = []diagnostic.Label{sourceLabel(extra, labelExtraArgument)}
		}
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:     item.Spans[0].File,
		Span:     diagnosticSpanFromJudgment(item.Spans[0]),
		Code:     code,
		Severity: severity,
		Message:  callArityMismatchMessage(name, detail.ExpectedCount, detail.ActualCount),
		Help:     callArityHelp(detail.ExpectedCount, detail.ActualCount),
		Labels:   labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    diagnosticSpanFromJudgment(item.Spans[0]),
				Message: callArgumentCountEvidence(name, detail.ActualCount),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
				Span:    directCallArityEvidenceSpan(item, judgment.EvidenceUserAssertion),
				Message: callParameterCountEvidence(name, detail.ExpectedCount),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustRefuted),
				Span:    directCallArityMissingProofSpan(item),
				Message: directCallArityMissingProofMessage(detail),
			},
		),
	}), true
}

func directCallArityDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceMissingProof {
			continue
		}
		if evidence.Detail.Kind == judgment.EvidenceDetailArityTooFew || evidence.Detail.Kind == judgment.EvidenceDetailArityTooMany {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func directCallArityEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind) diagnostic.Span {
	for _, evidence := range item.Evidence {
		if evidence.Kind == kind && evidence.Span.StartLine != 0 {
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
