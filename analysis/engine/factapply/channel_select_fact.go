package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

func channelSelectKind(kind factflow.ChannelSelectKind) (channelselectfact.Kind, bool) {
	switch kind {
	case factflow.ChannelSelectSelect:
		return channelselectfact.FactSelect, true
	case factflow.ChannelSelectReceive:
		return channelselectfact.FactReceive, true
	case factflow.ChannelSelectCase:
		return channelselectfact.FactCase, true
	default:
		return 0, false
	}
}

func applyNormalReturnChannelSelects(ctx normalReturnApplyContext, out state.State) state.State {
	for _, event := range ctx.normalFacts.ChannelSelects {
		fact, ok := callChannelSelectFactAt(ctx, event)
		if !ok {
			continue
		}
		out = out.AddChannelSelectFact(fact)
	}
	return out
}

func callChannelSelectFactAt(
	ctx normalReturnApplyContext,
	event callboundary.ChannelSelectFact,
) (channelselectfact.Fact, bool) {
	switch event.Kind {
	case channelselectfact.FactSelect, channelselectfact.FactReceive, channelselectfact.FactCase:
	default:
		return channelselectfact.Fact{}, false
	}
	fact := channelselectfact.Fact{
		Select:     event.Select,
		Kind:       event.Kind,
		Index:      event.Index,
		HasDefault: event.HasDefault,
	}
	if !event.Result.IsEmpty() {
		resultStateKey, ok := ctx.stateKey(event.Result)
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Result = resultStateKey
	}
	if !event.Case.IsEmpty() {
		caseStateKey, ok := ctx.stateKey(event.Case)
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Case = caseStateKey
	}
	return fact, true
}
