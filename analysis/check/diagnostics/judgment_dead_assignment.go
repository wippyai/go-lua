package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderDeadAssignmentJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeDeadAssignment || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
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
	hasExit := deadAssignmentJudgmentHasExit(item)
	evidence, labels := deadAssignmentJudgmentEvidence(item, span, name)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeDeadAssignment,
		Severity:    severity,
		Message:     deadAssignmentMessage(name, hasExit),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        deadAssignmentHelp(name, hasExit),
		Labels:      labels,
	}), true
}

func deadAssignmentJudgmentHasExit(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailDeadAssignmentExit {
			return true
		}
	}
	return false
}

func deadAssignmentJudgmentEvidence(item judgment.Judgment, primary diagnostic.Span, name string) ([]diagnostic.Evidence, []diagnostic.Label) {
	var evidence []diagnostic.Evidence
	labels := []diagnostic.Label{sourceLabel(primary, labelDeadAssignment)}
	for _, itemEvidence := range item.Evidence {
		span := diagnosticSpanFromJudgment(itemEvidence.Span)
		switch itemEvidence.Detail.Kind {
		case judgment.EvidenceDetailDeadAssignmentOverwrite:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: deadAssignmentOverwriteEvidence(name),
			})
			labels = append(labels, sourceLabel(span, labelOverwrite))
		case judgment.EvidenceDetailDeadAssignmentExit:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: deadAssignmentExitEvidence(name),
			})
			labels = append(labels, sourceLabel(span, labelExitBeforeRead))
		}
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		return judgmentDiagnosticEvidenceSpanLess(evidence[i].Span, evidence[j].Span)
	})
	return evidence, labels
}

func judgmentDiagnosticEvidenceSpanLess(left, right diagnostic.Span) bool {
	if left.Valid() != right.Valid() {
		return left.Valid()
	}
	if !left.Valid() {
		return false
	}
	if left.StartLine != right.StartLine {
		return left.StartLine < right.StartLine
	}
	if left.StartCol != right.StartCol {
		return left.StartCol < right.StartCol
	}
	if left.EndLine != right.EndLine {
		return left.EndLine < right.EndLine
	}
	return left.EndCol < right.EndCol
}
