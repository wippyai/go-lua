package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// FrozenTableMutations emits obligations for writes or calls that mutate a table
// identity already proved frozen at the mutation point.
type FrozenTableMutations struct{}

func (FrozenTableMutations) Name() string {
	return "effect.freeze.mutation"
}

func (FrozenTableMutations) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachFrozenTableMutation(func(mutation readmodel.FrozenTableMutation) bool {
		out = append(out, frozenTableMutationJudgment(ctx, functionKey, mutation))
		return true
	})
	return out
}

func frozenTableMutationJudgment(ctx Context, functionKey string, mutation readmodel.FrozenTableMutation) judgment.Judgment {
	label := mutation.ContainerLabel
	if label == "" {
		label = "table"
	}
	detail := judgment.EvidenceDetail{Kind: judgment.EvidenceDetailFrozenTableAssignment}
	if mutation.Kind == readmodel.FrozenTableMutationCall {
		detail.Kind = judgment.EvidenceDetailFrozenTableCall
	}
	evidence := judgment.EvidenceChain{
		{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: detail,
			Origin: judgment.OriginRef{
				Point: mutation.Point,
				Key:   judgment.OriginFrozenTableMutation,
			},
			Span: spanFromReadModel(ctx.SourceFile, mutation.MutationSpan),
		},
	}
	if mutation.HasFreezeProofSpan {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: mutation.Point,
				Key:   judgment.OriginFrozenTableProof,
			},
			Span: spanFromReadModel(ctx.SourceFile, mutation.FreezeProofSpan),
		})
	}
	return judgment.Judgment{
		Code:  judgment.CodeFrozenTable,
		Point: mutation.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("frozen-table:%d:%s", mutation.Point, mutation.ContainerKey),
		).WithLabel(label),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, mutation.MutationSpan)},
	}
}
