package factapply

import (
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
		fact.Result = factPathKeyAt(resolver, point, resultPath)
		if fact.Result == "" {
			return channelselectfact.Fact{}, false
		}
	}
	if casePath, ok := event.CasePath(); ok {
		fact.Case = factPathKeyAt(resolver, point, casePath)
		if fact.Case == "" {
			return channelselectfact.Fact{}, false
		}
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
