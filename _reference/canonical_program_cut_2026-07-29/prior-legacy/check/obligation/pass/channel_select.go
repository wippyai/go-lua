package pass

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ChannelSelects emits exhaustiveness obligations for channel.select result
// branch chains that do not cover every case.
type ChannelSelects struct{}

func (ChannelSelects) Name() string {
	return "channel.select.exhaustiveness"
}

func (ChannelSelects) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachChannelSelectExhaustiveness(func(item readmodel.ChannelSelectExhaustiveness) bool {
		out = append(out, channelSelectJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func channelSelectJudgment(ctx Context, functionKey string, item readmodel.ChannelSelectExhaustiveness) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, item.Span)
	evidence := judgment.EvidenceChain{
		{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailChannelSelectResult,
				SubjectLabel: item.ResultChannel,
			},
			Origin: judgment.OriginRef{Point: item.Point, Key: "channel-select:result"},
			Span:   span,
		},
	}
	if len(item.Handled) > 0 {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind:     judgment.EvidenceDetailChannelSelectHandled,
				CaseList: channelSelectCaseListKey(item.Handled),
			},
			Origin: judgment.OriginRef{Point: item.Point, Key: "channel-select:handled"},
			Span:   span,
		})
	}
	evidence = append(evidence, judgment.Evidence{
		Kind:  judgment.EvidenceMissingProof,
		Trust: judgment.EvidenceTrustUnknown,
		Detail: judgment.EvidenceDetail{
			Kind:     judgment.EvidenceDetailChannelSelectMissing,
			CaseList: channelSelectCaseListKey(item.Missing),
		},
		Origin: judgment.OriginRef{Point: item.Point, Key: "channel-select:missing"},
		Span:   span,
	})
	if !item.HasDefault {
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceMissingProof,
			Trust:  judgment.EvidenceTrustUnknown,
			Detail: judgment.EvidenceDetail{Kind: judgment.EvidenceDetailChannelSelectNoDefault},
			Origin: judgment.OriginRef{Point: item.Point, Key: "channel-select:default"},
			Span:   span,
		})
	}
	return judgment.Judgment{
		Code:  judgment.CodeChannelSelect,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("channel-select:%d:%s", item.Point, item.ResultChannel),
		).WithLabel(item.ResultChannel),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{span},
	}
}

func channelSelectCaseListKey(cases []string) string {
	return strings.Join(cases, "\x1f")
}
