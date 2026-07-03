package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceChannelSelectJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.ChannelSelects{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderChannelSelectJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeChannelSelect || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	missing := channelSelectJudgmentCases(item, judgment.EvidenceDetailChannelSelectMissing)
	if len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	caseWord := pluralize(len(missing), "case", "cases")
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeChannelSelectExhaustive,
		Severity:    severity,
		Message:     channelSelectExhaustivenessMessage(caseWord, channelCaseList(missing)),
		Explanation: channelSelectJudgmentExplanation(item, span),
		Help:        channelSelectExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelChannelCaseTest)},
	}), true
}

func channelSelectJudgmentExplanation(item judgment.Judgment, fallback diagnostic.Span) diagnostic.Explanation {
	var evidence []diagnostic.Evidence
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		if !span.Valid() {
			span = fallback
		}
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailChannelSelectResult:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: selectedChannelPathEvidence(itemEvidence.Detail.SubjectLabel),
			})
		case judgment.EvidenceDetailChannelSelectHandled:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: handledChannelCasesEvidence(channelCaseList(channelSelectCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailChannelSelectMissing:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingChannelCasesEvidence(channelCaseList(channelSelectCaseListFromKey(itemEvidence.Detail.CaseList))),
			})
		case judgment.EvidenceDetailChannelSelectNoDefault:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustUnknown),
				Span:    span,
				Message: missingChannelDefaultEvidence(),
			})
		}
	}
	return diagnostic.NewExplanation(evidence...)
}

func channelSelectJudgmentCases(item judgment.Judgment, kind judgment.EvidenceDetailKind) []string {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == kind {
			return channelSelectCaseListFromKey(evidence.Detail.CaseList)
		}
	}
	return nil
}

func channelSelectCaseListFromKey(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, "\x1f")
}
