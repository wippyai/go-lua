package pass

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// DiscriminatedUnions emits exhaustiveness obligations for if/elseif chains
// over discriminated union cases.
type DiscriminatedUnions struct{}

func (DiscriminatedUnions) Name() string {
	return "union.discriminated.exhaustiveness"
}

func (DiscriminatedUnions) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachDiscriminatedUnionExhaustiveness(func(item readmodel.DiscriminatedUnionExhaustiveness) bool {
		out = append(out, discriminatedUnionJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func discriminatedUnionJudgment(ctx Context, functionKey string, item readmodel.DiscriminatedUnionExhaustiveness) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, item.Span)
	evidence := judgment.EvidenceChain{
		{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: judgment.DiscriminatedUnionTargetEvidenceDetail(item.Target),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "union:target",
			},
			Span: span,
		},
		{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: judgment.DiscriminatedUnionPossibleEvidenceDetail(discriminatedUnionCaseListKey(item.Possible)),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "union:possible",
			},
			Span: span,
		},
	}
	if len(item.Handled) > 0 {
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: judgment.DiscriminatedUnionHandledEvidenceDetail(discriminatedUnionCaseListKey(item.Handled)),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "union:handled",
			},
			Span: span,
		})
	}
	evidence = append(evidence,
		judgment.Evidence{
			Kind:   judgment.EvidenceMissingProof,
			Trust:  judgment.EvidenceTrustUnknown,
			Detail: judgment.DiscriminatedUnionMissingEvidenceDetail(discriminatedUnionCaseListKey(item.Missing)),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "union:missing",
			},
			Span: span,
		},
		judgment.Evidence{
			Kind:   judgment.EvidenceMissingProof,
			Trust:  judgment.EvidenceTrustUnknown,
			Detail: judgment.DiscriminatedUnionNoDefaultEvidenceDetail(),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "union:default",
			},
			Span: span,
		},
	)
	return judgment.Judgment{
		Code:  judgment.CodeDiscriminatedUnion,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("discriminated-union:%d:%s", item.Point, item.Target),
		).WithLabel(item.Target),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{span},
	}
}

func discriminatedUnionCaseListKey(cases []string) string {
	return strings.Join(cases, "\x1f")
}
