package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func firstDirectCallContractJudgmentPerCall(groups ...[]judgment.Judgment) []judgment.Judgment {
	var out []judgment.Judgment
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, item := range group {
			key := fmt.Sprintf("%s|%d", item.Subject.FunctionKey, item.Point)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func renderDirectCallArgumentJudgment(item judgment.Judgment) (diagnostic.Diagnostic, bool) {
	return renderDirectCallArgumentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func renderDirectCallArgumentJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeCallArgType || item.Subject.Kind != judgment.SubjectCallArgument || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	presentation, ok := diagnosticProofContext().DirectCallArgument(item, span)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeDirectCallArgType,
		Severity:    severity,
		Message:     presentation.Message,
		Explanation: diagnostic.NewExplanation(presentation.Evidence...),
		Help:        presentation.Help,
		Labels:      presentation.Labels,
	}), true
}
