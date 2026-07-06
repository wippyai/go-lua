package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
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
		Message: missingMemberMessage(receiver, detail.Field),
		Help:    missingMemberHelp(detail.Field),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelMemberRead)},
		Evidence: []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
				Span:    primary,
				Message: memberReadReceiverEvidence(readPath, detail.Field, receiver),
			},
		},
	}, true
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
		Message: concatOperandMessage(detail.Field),
		Help:    concatOperandHelp(operandName),
		Labels:  []diagnostic.Label{sourceLabel(primary, labelValueMayBeNil)},
		Evidence: []diagnostic.Evidence{
			{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Reason:  concatOperandEvidenceReason(got),
				Span:    primary,
				Message: concatOperandTypeEvidence(detail.Field, operandName, got),
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
