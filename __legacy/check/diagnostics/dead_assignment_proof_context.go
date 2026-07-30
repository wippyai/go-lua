package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type deadAssignmentPresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) DeadAssignment(item judgment.Judgment, primary diagnostic.Span, name string) deadAssignmentPresentation {
	hasExit := deadAssignmentHasExit(item)
	evidence, labels := deadAssignmentEvidence(item, primary, name)
	return deadAssignmentPresentation{
		Message:  display.DeadAssignmentMessage(name, hasExit),
		Help:     display.DeadAssignmentHelp(name, hasExit),
		Evidence: evidence,
		Labels:   labels,
	}
}

func deadAssignmentHasExit(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailDeadAssignmentExit {
			return true
		}
	}
	return false
}

func deadAssignmentEvidence(item judgment.Judgment, primary diagnostic.Span, name string) ([]diagnostic.Evidence, []diagnostic.Label) {
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
				Message: display.DeadAssignmentOverwriteEvidence(name),
			})
			labels = append(labels, sourceLabel(span, labelOverwrite))
		case judgment.EvidenceDetailDeadAssignmentExit:
			evidence = append(evidence, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentTrust(itemEvidence.Trust, diagnostic.TrustProven),
				Span:    span,
				Message: display.DeadAssignmentExitEvidence(name),
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
