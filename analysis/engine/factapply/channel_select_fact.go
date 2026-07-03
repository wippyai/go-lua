package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyChannelSelect(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	event factflow.ChannelSelect,
) state.State {
	fact, ok := channelSelectFactAt(resolver, ctx.Point, event)
	if !ok {
		return out
	}
	return out.AddChannelSelectFact(fact)
}

func channelSelectFactAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	event factflow.ChannelSelect,
) (channelselectfact.Fact, bool) {
	kind, ok := channelSelectKind(event.Kind())
	if !ok {
		return channelselectfact.Fact{}, false
	}
	fact := channelselectfact.Fact{
		Select:     channelselectfact.ID(event.SelectID()),
		Kind:       kind,
		Index:      event.Index(),
		HasDefault: event.HasDefault(),
	}
	if resultPath, ok := event.ResultPath(); ok {
		resultKey, ok := visibility.AddressAt(resolver, point, resultPath).VisibleStateKey()
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Result = resultKey
	}
	if casePath, ok := event.CasePath(); ok {
		caseKey, ok := visibility.AddressAt(resolver, point, casePath).VisibleStateKey()
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Case = caseKey
	}
	if payload, ok := event.PayloadValue(); ok {
		fact.Payload = payload
		fact.HasPayload = true
	}
	return fact, true
}

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
