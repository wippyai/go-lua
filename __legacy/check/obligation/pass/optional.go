package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// Optionals emits exhaustiveness obligations for optional nil/value branches.
type Optionals struct{}

func (Optionals) Name() string {
	return "union.optional.exhaustiveness"
}

func (Optionals) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachOptionalExhaustiveness(func(item readmodel.OptionalExhaustiveness) bool {
		out = append(out, optionalJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func optionalJudgment(ctx Context, functionKey string, item readmodel.OptionalExhaustiveness) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, item.Span)
	return judgment.Judgment{
		Code:  judgment.CodeOptional,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("optional:%d:%s", item.Point, item.Target),
		).WithLabel(item.Target),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.OptionalTargetEvidenceDetail(item.Target),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "optional:target",
				},
				Span: span,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.OptionalPossibleEvidenceDetail(item.Target),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "optional:possible",
				},
				Span: span,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.OptionalConsumedEvidenceDetail(item.Target),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "optional:consumed",
				},
				Span: span,
			},
			{
				Kind:   judgment.EvidenceMissingProof,
				Trust:  judgment.EvidenceTrustUnknown,
				Detail: judgment.OptionalMissingEvidenceDetail(discriminatedUnionCaseListKey(item.Missing)),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "optional:missing",
				},
				Span: span,
			},
			{
				Kind:   judgment.EvidenceMissingProof,
				Trust:  judgment.EvidenceTrustUnknown,
				Detail: judgment.OptionalNoDefaultEvidenceDetail(),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "optional:default",
				},
				Span: span,
			},
		},
		Spans: []judgment.SpanRef{span},
	}
}
