package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ChannelLifecycles emits runtime channel lifecycle misuse judgments.
type ChannelLifecycles struct{}

func (ChannelLifecycles) Name() string {
	return "channel.lifecycle"
}

func (ChannelLifecycles) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachChannelLifecycleMisuse(func(item readmodel.ChannelLifecycleMisuse) bool {
		out = append(out, channelLifecycleJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func channelLifecycleJudgment(ctx Context, functionKey string, item readmodel.ChannelLifecycleMisuse) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, item.Span)
	code := judgment.CodeChannelSendClosed
	if item.Operation == readmodel.ChannelLifecycleClose {
		code = judgment.CodeChannelDoubleClose
	}
	operation := string(item.Operation)
	return judgment.Judgment{
		Code:  code,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("channel-lifecycle:%d:%s:%s", item.Point, operation, item.Channel),
		).WithLabel(item.Channel),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailChannelClosed,
				SubjectLabel: item.Channel,
				Field:        operation,
				Message:      item.State,
			},
			Origin: judgment.OriginRef{Point: item.Point, Key: "channel-lifecycle:closed"},
			Span:   span,
		}},
		Spans: []judgment.SpanRef{span},
	}
}
