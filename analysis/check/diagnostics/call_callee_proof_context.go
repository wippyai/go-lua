package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type callCalleePresentation struct {
	Code     diagnostic.Code
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Label    string
}

func (ProofContext) DirectCallCallee(item judgment.Judgment, primary diagnostic.Span) (callCalleePresentation, bool) {
	proof := item.CallCalleeProof()
	if !proof.Found {
		return callCalleePresentation{}, false
	}
	detail := proof.Detail
	name := item.Subject.Label
	if name == "" {
		name = "call target"
	}
	if exact, fieldUse := exactNilabilityUse(item); exact != "" {
		name = exact
		help := fmt.Sprintf("Guard %s against nil before the method call, or assign it on every branch.", name)
		label := "possibly-nil value"
		if fieldUse {
			help = fmt.Sprintf("Guard %s against nil before the method call.", name)
			label = "possibly-nil field"
		}
		return callCalleePresentation{
			Code:     CodeNilUnsafeUse,
			Message:  fmt.Sprintf("%s may be nil at method call", name),
			Help:     help,
			Evidence: nilabilityProvenanceEvidence(item, name, item.Actual.ProjectedType, primary),
			Label:    label,
		}, true
	}
	message := display.DirectNotCallableMessage(name, item.Actual.ProjectedType)
	help := display.DirectNotCallableHelp(name)
	if detail.Kind == judgment.EvidenceDetailMemberMissing {
		message = display.MissingMemberMessage(item.Actual.ProjectedType, detail.Field)
		help = missingMemberHelp(item.Actual.ProjectedType, detail.Field)
	}
	if detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		message = display.PossiblyNilCallTargetMessage(name)
		help = display.PossiblyNilCallTargetHelp(name)
		if detail.MemberAccess && !detail.Callable {
			receiverName, callName := memberCalleeOptionalNames(name)
			message = display.OptionalMethodCallMessage()
			help = display.OptionalMethodCallHelp(receiverName, callName)
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
	evidence := callCalleeJudgmentEvidence(item, detail, name, primary)
	if detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		evidence = append(evidence, nilabilityProvenanceEvidence(item, name, item.Actual.ProjectedType, primary)...)
	}
	return callCalleePresentation{
		Code:     code,
		Message:  message,
		Help:     help,
		Evidence: evidence,
		Label:    label,
	}, true
}

func exactNilabilityUse(item judgment.Judgment) (string, bool) {
	for _, evidence := range item.Evidence {
		detail := evidence.Detail
		if detail.Kind == judgment.EvidenceDetailMayBeNil &&
			detail.Cause.Kind == judgment.EvidenceCauseUse &&
			detail.SubjectLabel != "" {
			return detail.SubjectLabel, detail.MemberAccess
		}
	}
	return "", false
}

func callCalleeJudgmentEvidence(item judgment.Judgment, detail judgment.EvidenceDetail, name string, primary diagnostic.Span) []diagnostic.Evidence {
	actual := item.Actual.ProjectedType
	if detail.Kind == judgment.EvidenceDetailMemberMissing {
		return []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Cause:   diagnosticCauseFromJudgmentDetail(detail),
				Span:    primary,
				Message: display.ReceiverForMemberEvidence(name, actual),
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
					Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceAbstractFact),
					Span:    primary,
					Message: display.OptionalMethodReceiverEvidence(subject, target),
				},
				{
					Kind:    diagnostic.EvidenceMissingProof,
					Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustUnknown),
					Cause:   diagnosticCauseFromJudgmentDetail(detail),
					Span:    primary,
					Message: display.OptionalMethodMissingNilCheckEvidence(subject, callTarget),
				},
			}
		}
		if detail.MemberAccess && detail.Callable {
			return []diagnostic.Evidence{
				{
					Kind:    diagnostic.EvidenceAbstractFact,
					Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
					Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceAbstractFact),
					Span:    primary,
					Message: display.MemberTypeAtCallEvidence(name, actual),
				},
				{
					Kind:    diagnostic.EvidenceUserAssertion,
					Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
					Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceUserAssertion),
					Span:    primary,
					Message: fmt.Sprintf("%s must be non-nil before it is called", name),
				},
				{
					Kind:    diagnostic.EvidenceMissingProof,
					Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustUnknown),
					Cause:   diagnosticCauseFromJudgmentDetail(detail),
					Span:    primary,
					Message: display.MissingNonNilBeforeCallMessage(name),
				},
			}
		}
		actualMessage := display.PossiblyNilCalleeTypeEvidence(name, actual, detail.Callable)
		return []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceAbstractFact),
				Span:    primary,
				Message: actualMessage,
			},
			{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
				Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceUserAssertion),
				Span:    primary,
				Message: fmt.Sprintf("%s must be non-nil before it is called", name),
			},
			{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustUnknown),
				Cause:   diagnosticCauseFromJudgmentDetail(detail),
				Span:    primary,
				Message: display.MissingNonNilBeforeCallMessage(name),
			},
		}
	}
	actualMessage := display.SourceTypeEvidence(name, actual)
	if detail.MemberAccess {
		actualMessage = fmt.Sprintf("%s has type %s at call", name, diagnosticDisplay{}.Type(actual))
	}
	return []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceAbstractFact),
			Span:    primary,
			Message: actualMessage,
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
			Cause:   diagnosticCauseForJudgmentEvidenceKind(item, judgment.EvidenceUserAssertion),
			Span:    primary,
			Message: fmt.Sprintf("%s must be callable before it is called", name),
		},
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, diagnostic.TrustRefuted),
			Cause:   diagnosticCauseFromJudgmentDetail(detail),
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
