package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// MemberReads emits obligations for static member reads whose receiver is known
// to reject the requested member on the solved path.
type MemberReads struct{}

func (MemberReads) Name() string {
	return "member.read"
}

func (MemberReads) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachMissingMemberRead(func(read readmodel.MissingMemberRead) bool {
		out = append(out, memberReadJudgment(ctx, functionKey, read))
		return true
	})
	return out
}

func memberReadJudgment(ctx Context, functionKey string, read readmodel.MissingMemberRead) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, read.Span)
	detail := judgment.MemberMissingEvidenceDetail(read.MemberName)
	return judgment.Judgment{
		Code:  judgment.CodeMemberRead,
		Point: read.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("member-read:%d:%s", read.Point, read.ReadLabel),
		).WithLabel(read.ReadLabel),
		Actual:  judgment.NewValueRef(0, read.ReceiverType),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{
					Point: read.Point,
					Key:   "member-read:receiver",
				},
				Detail: detail,
				Span:   span,
			},
			{
				Kind:  judgment.EvidenceMissingProof,
				Trust: judgment.EvidenceTrustRefuted,
				Origin: judgment.OriginRef{
					Point: read.Point,
					Key:   "member-read:proof",
				},
				Detail: detail,
				Span:   span,
			},
		},
		Spans: []judgment.SpanRef{span},
	}
}
