package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceUnresolvedValueJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.UnresolvedValues{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderUnresolvedValueJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeUnresolvedValue || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	name := item.Subject.Label
	if name == "" {
		name = "<missing>"
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeUnresolvedValueReference,
		Severity:    severity,
		Message:     unresolvedValueMessage(name),
		Explanation: unresolvedValueJudgmentExplanation(item, name, span),
		Help:        unresolvedValueHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelUnknownValue)},
	}), true
}

func unresolvedValueJudgmentExplanation(item judgment.Judgment, name string, span diagnostic.Span) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    span,
			Message: unresolvedValueEvidence(name),
		},
	)
}
