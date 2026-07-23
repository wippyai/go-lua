package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// UnresolvedValues emits obligations for identifier reads that binding left as
// implicit globals in the current scope.
type UnresolvedValues struct{}

func (UnresolvedValues) Name() string {
	return "value.reference.unresolved"
}

func (UnresolvedValues) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachUnresolvedValueReference(func(ref readmodel.UnresolvedValueReference) bool {
		out = append(out, unresolvedValueJudgment(ctx, functionKey, ref))
		return true
	})
	return out
}

func unresolvedValueJudgment(ctx Context, functionKey string, ref readmodel.UnresolvedValueReference) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, ref.Span)
	return judgment.Judgment{
		Code:  judgment.CodeUnresolvedValue,
		Point: ref.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("unresolved-value:%d:%s", ref.Point, ref.Key),
		).WithLabel(ref.Name),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{
					Point: ref.Point,
					Key:   "unresolved-value:lookup",
				},
				Span: span,
			},
		},
		Spans: []judgment.SpanRef{span},
	}
}
