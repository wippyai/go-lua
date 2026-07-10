package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ResultShapes emits exhaustiveness obligations for reads of fields that exist
// on only one case of a discriminated union.
type ResultShapes struct{}

func (ResultShapes) Name() string {
	return "union.result_shape.exhaustiveness"
}

func (ResultShapes) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachResultShapeExhaustiveness(func(item readmodel.ResultShapeExhaustiveness) bool {
		out = append(out, resultShapeJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func resultShapeJudgment(ctx Context, functionKey string, item readmodel.ResultShapeExhaustiveness) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, item.Span)
	return judgment.Judgment{
		Code:  judgment.CodeResultShape,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("result-shape:%d:%s", item.Point, item.ReadLabel),
		).WithLabel(item.ReadLabel),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.ResultShapeUnionEvidenceDetail(item.ReceiverLabel, item.Discriminant),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "result-shape:union",
				},
				Span: span,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.ResultShapeFieldCaseEvidenceDetail(item.ReadLabel, item.RequiredCase),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "result-shape:case",
				},
				Span: span,
			},
			{
				Kind:   judgment.EvidenceMissingProof,
				Trust:  judgment.EvidenceTrustUnknown,
				Detail: judgment.ResultShapeMissingProofEvidenceDetail(item.RequiredCase),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "result-shape:proof",
				},
				Span: span,
			},
		},
		Spans: []judgment.SpanRef{span},
	}
}
