package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceUnusedLocalJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.UnusedLocals{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderUnusedLocalJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeUnusedLocal || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	name := item.Subject.Label
	if name == "" {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeUnusedLocal,
		Severity:    severity,
		Message:     unusedLocalMessage(name),
		Explanation: unusedLocalJudgmentExplanation(item, name),
		Help:        unusedLocalHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelUnusedLocal)},
	}), true
}

func unusedLocalJudgmentExplanation(item judgment.Judgment, name string) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Message: unusedLocalEvidence(name),
		},
	)
}
