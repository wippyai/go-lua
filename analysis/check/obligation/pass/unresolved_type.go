package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// UnresolvedTypes emits obligations for annotation type names that binding left
// unresolved in the current lexical/module type namespace.
type UnresolvedTypes struct{}

func (UnresolvedTypes) Name() string {
	return "type.reference.unresolved"
}

func (UnresolvedTypes) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachUnresolvedTypeReference(func(ref readmodel.UnresolvedTypeReference) bool {
		out = append(out, unresolvedTypeJudgment(ctx, functionKey, ref))
		return true
	})
	return out
}

func unresolvedTypeJudgment(ctx Context, functionKey string, ref readmodel.UnresolvedTypeReference) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, ref.Span)
	return judgment.Judgment{
		Code:  judgment.CodeUnresolvedType,
		Point: ref.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("unresolved-type:%d:%s", ref.Point, ref.Key),
		).WithLabel(ref.Name),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{
					Point: ref.Point,
					Key:   "unresolved-type:lookup",
				},
				Span: span,
			},
		},
		Spans: []judgment.SpanRef{span},
	}
}
