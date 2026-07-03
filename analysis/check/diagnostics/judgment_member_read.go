package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceMemberReadJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	query := newDiagnosticQuery(result)
	items := pass.New(pass.MemberReads{}).Run(pass.Context{
		FunctionKey:    sourceFile,
		SourceFile:     sourceFile,
		Reader:         query.reader,
		PointReachable: result.PointNormallyReachable,
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func renderMemberReadJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
	if item.Code != judgment.CodeMemberRead || item.Subject.Kind != judgment.SubjectExpression || len(item.Spans) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	severity, ok := diagnosticSeverityForJudgment(item, policy, mode)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	detail, ok := memberReadJudgmentDetail(item)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	span := diagnosticSpanFromJudgment(item.Spans[0])
	readPath := item.Subject.Label
	if readPath == "" {
		readPath = "member read"
	}
	receiver := item.Actual.ProjectedType
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:     item.Spans[0].File,
		Span:     span,
		Code:     CodeMissingMember,
		Severity: severity,
		Message:  missingMemberMessage(receiver, detail.Field),
		Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    span,
			Message: memberReadReceiverEvidence(readPath, detail.Field, receiver),
		}),
		Help:   missingMemberHelp(detail.Field),
		Labels: []diagnostic.Label{sourceLabel(span, labelMemberRead)},
	}), true
}

func memberReadJudgmentDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailMemberMissing && evidence.Detail.Field != "" {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}
