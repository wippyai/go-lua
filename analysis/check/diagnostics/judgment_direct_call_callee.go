package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceDirectCallCalleeJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.DirectCallCallee{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func renderDirectCallCalleeJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeCallCallee || item.Subject.Kind != judgment.SubjectCallExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	detail, ok := directCallCalleeDetail(item)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	name := item.Subject.Label
	if name == "" {
		name = "call target"
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	message := directNotCallableMessage(name, item.Actual.ProjectedType)
	help := directNotCallableHelp(name)
	if detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		message = possiblyNilCallTargetMessage(name)
		help = possiblyNilCallTargetHelp(name)
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeDirectCallNotCallable,
		Severity:    severity,
		Message:     message,
		Explanation: diagnostic.NewExplanation(directCallCalleeJudgmentEvidence(item, detail, name, span)...),
		Help:        help,
		Labels:      []diagnostic.Label{sourceLabel(span, labelCallTarget)},
	}), true
}

func directCallCalleeDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceMissingProof {
			continue
		}
		if evidence.Detail.Kind == judgment.EvidenceDetailCalleeNotCallable || evidence.Detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func directCallCalleeJudgmentEvidence(item judgment.Judgment, detail judgment.EvidenceDetail, name string, primary diagnostic.Span) []diagnostic.Evidence {
	actual := item.Actual.ProjectedType
	if detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		return []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: possiblyNilCalleeTypeEvidence(name, actual, detail.Callable),
			},
			{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
				Span:    primary,
				Message: fmt.Sprintf("%s must be non-nil before it is called", name),
			},
			{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustUnknown),
				Span:    primary,
				Message: missingNonNilBeforeCallMessage(name),
			},
		}
	}
	return []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    primary,
			Message: assignmentSourceTypeEvidence(name, actual),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
			Span:    primary,
			Message: fmt.Sprintf("%s must be callable before it is called", name),
		},
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustRefuted),
			Span:    primary,
			Message: fmt.Sprintf("no proof on this path shows %s is callable", name),
		},
	}
}
