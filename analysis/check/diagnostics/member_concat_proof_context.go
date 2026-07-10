package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/internal/callcontract"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type memberReadPresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

type concatOperandPresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

func (ProofContext) MemberRead(item judgment.Judgment, primary diagnostic.Span) (memberReadPresentation, bool) {
	proof := item.MemberReadProof()
	if !proof.Found {
		return memberReadPresentation{}, false
	}
	detail := proof.Detail
	readPath := item.Subject.Label
	if readPath == "" {
		readPath = "member read"
	}
	receiver := item.Actual.ProjectedType
	return memberReadPresentation{
		Message: display.MissingMemberMessage(receiver, detail.Field),
		Help:    missingMemberHelp(receiver, detail.Field),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelMemberRead)},
		Evidence: []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Cause:   diagnosticCauseFromJudgmentDetail(detail),
				Span:    primary,
				Message: display.MemberReadReceiverEvidence(readPath, detail.Field, receiver),
			},
		},
	}, true
}

func missingMemberHelp(receiver typ.Type, member string) string {
	help := display.MissingMemberHelp(member)
	if !missingMemberMatchesCallableMethod(receiver, member) {
		return help
	}
	hint := display.MissingMemberMethodHint(member)
	if hint == "" {
		return help
	}
	if strings.TrimSpace(help) == "" {
		return hint
	}
	return help + " " + hint
}

func missingMemberMatchesCallableMethod(receiver typ.Type, member string) bool {
	if receiver == nil || member == "" {
		return false
	}
	_, status, ok := callcontract.MemberCall(receiver, segment.Segment{
		Kind: segment.SegmentField,
		Name: member,
	})
	return ok && status == callcontract.MemberCallOK
}

func (ProofContext) ConcatOperand(item judgment.Judgment, primary diagnostic.Span) (concatOperandPresentation, bool) {
	proof := item.ConcatOperandProof()
	if !proof.Found {
		return concatOperandPresentation{}, false
	}
	detail := proof.Detail
	operandName := item.Subject.Label
	got := item.Actual.ProjectedType
	return concatOperandPresentation{
		Message: display.ConcatOperandMessage(detail.Field, operandName),
		Help:    display.ConcatOperandHelp(operandName),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelValueMayBeNil)},
		Evidence: []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Reason:  concatOperandEvidenceReason(got),
				Span:    primary,
				Message: display.ConcatOperandTypeEvidence(detail.Field, operandName, got),
			},
		},
	}, true
}

func concatOperandEvidenceReason(got typ.Type) diagnostic.EvidenceReason {
	if got != nil && typ.Nil.Equals(got) {
		return diagnostic.EvidenceReasonExactType
	}
	if readmodel.ProjectionHasNil(got) {
		return diagnostic.EvidenceReasonUnionType
	}
	return diagnostic.EvidenceReasonUnspecified
}
