package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func produceNumericForJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.NumericForOperands{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderNumericForJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeNumericForOperand || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got := item.Actual.ProjectedType
	if got == nil {
		return diagnostic.Diagnostic{}, false
	}
	role := item.Expected.Label
	if role == "" {
		role = item.Subject.Label
	}
	if role == "" {
		role = "operand"
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    numericForJudgmentEvidenceSpan(item, judgment.EvidenceAbstractFact, span),
			Message: numericForOperandTypeEvidence(role, got),
		},
	}
	if item.HasEvidence(judgment.EvidenceUserAssertion) {
		evidence = append(evidence, numericForJudgmentExplicitTopEvidence(item, span)...)
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeNumericForOperand,
		Severity:    severity,
		Message:     numericForOperandMessage(role, got),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        numericForOperandHelp(role),
		Labels:      []diagnostic.Label{sourceLabel(span, role)},
	}), true
}

func numericForJudgmentExplicitTopEvidence(item judgment.Judgment, span diagnostic.Span) []diagnostic.Evidence {
	subject := "assigned value"
	want := item.Expected.Type
	if want == nil {
		want = typ.Number
	}
	out := diagnostic.AssertionEvidence(span, assertion.Any())
	if item.HasEvidence(judgment.EvidencePrecisionBoundary) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidencePrecisionBoundary, diagnostic.TrustUnknown),
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Span:    numericForJudgmentEvidenceSpan(item, judgment.EvidencePrecisionBoundary, span),
			Message: explicitBoundaryProofMessageForSubject(subject, want),
		})
	}
	if item.HasEvidence(judgment.EvidenceMissingProof) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustUnknown),
			Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			Span:    numericForJudgmentEvidenceSpan(item, judgment.EvidenceMissingProof, span),
			Message: missingBoundaryProofMessageForSubject(subject, want),
		})
	}
	return out
}

func numericForJudgmentEvidenceSpan(item judgment.Judgment, kind judgment.EvidenceKind, fallback diagnostic.Span) diagnostic.Span {
	for _, evidence := range item.Evidence {
		if evidence.Kind == kind && evidence.Span.StartLine != 0 {
			return diagnosticSpanFromJudgment(evidence.Span)
		}
	}
	return fallback
}
