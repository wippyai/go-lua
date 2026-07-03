package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func renderCallCalleeJudgmentWithPolicy(item judgment.Judgment, policy judgment.Policy, mode judgment.StrictnessMode) (diagnostic.Diagnostic, bool) {
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
	if detail.Kind == judgment.EvidenceDetailMemberMissing {
		message = missingMemberMessage(item.Actual.ProjectedType, detail.Field)
		help = missingMemberHelp(detail.Field)
	}
	if detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		message = possiblyNilCallTargetMessage(name)
		help = possiblyNilCallTargetHelp(name)
		if detail.MemberAccess && !detail.Callable {
			receiverName, callName := memberCalleeOptionalNames(name)
			message = optionalMethodCallMessage()
			help = optionalMethodCallHelp(receiverName, callName)
		}
	}
	code := CodeDirectCallNotCallable
	label := labelCallTarget
	if detail.MemberAccess && detail.Kind == judgment.EvidenceDetailCalleeMayBeNil && !detail.Callable {
		code = CodeOptionalMethodCall
		label = labelMethodCall
	} else if detail.Kind == judgment.EvidenceDetailMemberMissing {
		code = CodeMissingMember
		label = labelMemberCall
	} else if detail.MemberAccess && detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		code = CodeNotCallable
		label = labelMemberCall
	} else if detail.MemberAccess {
		code = CodeNotCallable
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:        item.Spans[0].File,
		Span:        span,
		Code:        code,
		Severity:    severity,
		Message:     message,
		Explanation: diagnostic.NewExplanation(callCalleeJudgmentEvidence(item, detail, name, span)...),
		Help:        help,
		Labels:      []diagnostic.Label{sourceLabel(span, label)},
	}), true
}

func directCallCalleeDetail(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind != judgment.EvidenceMissingProof {
			continue
		}
		if evidence.Detail.Kind == judgment.EvidenceDetailCalleeNotCallable || evidence.Detail.Kind == judgment.EvidenceDetailCalleeMayBeNil || evidence.Detail.Kind == judgment.EvidenceDetailMemberMissing {
			return evidence.Detail, true
		}
	}
	return judgment.EvidenceDetail{}, false
}

func callCalleeJudgmentEvidence(item judgment.Judgment, detail judgment.EvidenceDetail, name string, primary diagnostic.Span) []diagnostic.Evidence {
	actual := item.Actual.ProjectedType
	if detail.Kind == judgment.EvidenceDetailMemberMissing {
		return []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: receiverForMemberEvidence(name, actual),
			},
		}
	}
	if detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		if detail.MemberAccess && !detail.Callable {
			receiverName, callName := memberCalleeOptionalNames(name)
			subject := "receiver"
			if receiverName != "" {
				subject = "receiver " + receiverName
			}
			target := ""
			if callName != "" {
				target = " at call to " + callName
			}
			callTarget := "this method call"
			if callName != "" {
				callTarget = "calling " + callName
			}
			return []diagnostic.Evidence{
				{
					Kind:    diagnostic.EvidenceAbstractFact,
					Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
					Span:    primary,
					Message: optionalMethodReceiverEvidence(subject, target),
				},
				{
					Kind:    diagnostic.EvidenceMissingProof,
					Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustUnknown),
					Span:    primary,
					Message: optionalMethodMissingNilCheckEvidence(subject, callTarget),
				},
			}
		}
		if detail.MemberAccess && detail.Callable {
			return []diagnostic.Evidence{
				{
					Kind:    diagnostic.EvidenceAbstractFact,
					Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
					Span:    primary,
					Message: memberTypeAtCallEvidence(name, actual),
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
		actualMessage := possiblyNilCalleeTypeEvidence(name, actual, detail.Callable)
		return []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: actualMessage,
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
	actualMessage := assignmentSourceTypeEvidence(name, actual)
	if detail.MemberAccess {
		actualMessage = fmt.Sprintf("%s has type %s at call", name, diagnosticDisplay{}.Type(actual))
	}
	return []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    primary,
			Message: actualMessage,
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

func memberCalleeOptionalNames(name string) (receiverName string, callName string) {
	callName = name
	if dot := strings.LastIndex(name, "."); dot > 0 {
		receiverName = name[:dot]
	}
	return receiverName, callName
}
