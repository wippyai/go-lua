package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceUnresolvedTypeJudgmentDiagnosticsWithPolicy(result, parent *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	reader := readmodel.NewWithParent(result, parent)
	items := pass.New(pass.UnresolvedTypes{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderUnresolvedTypeJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeUnresolvedType || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
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
		Code:        CodeUnresolvedTypeReference,
		Severity:    severity,
		Message:     unresolvedTypeMessage(name),
		Explanation: unresolvedTypeJudgmentExplanation(item, name, span),
		Help:        unresolvedTypeHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelUnknownType)},
	}), true
}

func unresolvedTypeJudgmentExplanation(item judgment.Judgment, name string, span diagnostic.Span) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    span,
			Message: unresolvedTypeEvidence(name),
		},
	)
}
