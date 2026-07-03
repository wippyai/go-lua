package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceFrozenTableJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.FrozenTableMutations{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderFrozenTableJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeFrozenTable || item.Subject.Kind != judgment.SubjectPath || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	containerName := item.Subject.Label
	if containerName == "" {
		containerName = "table"
	}
	mutatingCall := frozenTableJudgmentIsCall(item)
	evidence := frozenTableJudgmentEvidence(item, span, containerName, mutatingCall)
	labels := []diagnostic.Label{sourceLabel(span, labelFrozenTableMutation)}
	if mutatingCall {
		labels[0] = sourceLabel(span, labelFrozenTableCall)
	}
	if proofSpan, ok := frozenTableJudgmentProofSpan(item); ok {
		labels = append(labels, sourceLabel(proofSpan, labelFreezeProof))
	}
	message := frozenTableMutationMessage(containerName)
	help := frozenTableAssignmentHelp()
	if mutatingCall {
		message = frozenTableCallMutationMessage(containerName)
		help = frozenTableCallHelp()
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        CodeFrozenTableMutation,
		Message:     message,
		Severity:    severity,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        help,
		Labels:      labels,
	}), true
}

func frozenTableJudgmentIsCall(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailFrozenTableCall {
			return true
		}
	}
	return false
}

func frozenTableJudgmentEvidence(item judgment.Judgment, span diagnostic.Span, containerName string, mutatingCall bool) []diagnostic.Evidence {
	primaryMessage := frozenAssignmentEvidence(containerName)
	incomingMessage := frozenIncomingStateEvidence(containerName)
	if mutatingCall {
		primaryMessage = frozenCallMutationEvidence(containerName)
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    span,
			Message: primaryMessage,
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Message: incomingMessage,
		},
	}
	if proofSpan, ok := frozenTableJudgmentProofSpan(item); ok {
		evidence[1].Span = proofSpan
		if mutatingCall {
			evidence[1].Message = frozenCallProofEvidence(containerName)
		} else {
			evidence[1].Message = frozenAssignmentProofEvidence(containerName)
		}
	}
	return evidence
}

func frozenTableJudgmentProofSpan(item judgment.Judgment) (diagnostic.Span, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Origin.Key == judgment.OriginFrozenTableProof && evidence.Span.StartLine != 0 {
			span := diagnosticSpanFromJudgment(evidence.Span)
			if span.Valid() {
				return span, true
			}
		}
	}
	return diagnostic.Span{}, false
}
