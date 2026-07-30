package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// UnusedLocals emits lint obligations for reachable local declarations whose
// symbols have no reachable read in their scope.
type UnusedLocals struct{}

func (UnusedLocals) Name() string {
	return "lint.unused.local"
}

func (UnusedLocals) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachUnusedLocal(func(local readmodel.UnusedLocal) bool {
		out = append(out, unusedLocalJudgment(ctx, functionKey, local))
		return true
	})
	return out
}

func unusedLocalJudgment(ctx Context, functionKey string, local readmodel.UnusedLocal) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, local.Span)
	return judgment.Judgment{
		Code:  judgment.CodeUnusedLocal,
		Point: local.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("unused-local:%d:%s", local.Point, local.Key),
		).WithLabel(local.Name),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{
					Point: local.Point,
					Key:   "unused-local:no-read",
				},
			},
		},
		Spans: []judgment.SpanRef{span},
	}
}
