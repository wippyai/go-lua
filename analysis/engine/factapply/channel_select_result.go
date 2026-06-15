package factapply

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/channelselect"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type channelSelectResultGroup struct {
	resultIndex int
	hasResult   bool
	cases       []factflow.ChannelSelect
}

func applyChannelSelectResult(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	events []factflow.ChannelSelect,
) state.State {
	groups := channelSelectResultGroups(events)
	for selectID, group := range groups {
		if !group.hasResult || group.resultIndex < 0 || len(group.cases) == 0 {
			continue
		}
		resultValue, ok := channelSelectResultValue(ctx, resolver, out, selectID, group.cases)
		if !ok {
			continue
		}
		out = out.WriteReturnSlot(ctx.Registry, group.resultIndex, resultValue)
	}
	return out
}

func channelSelectResultGroups(events []factflow.ChannelSelect) map[factflow.ChannelSelectID]channelSelectResultGroup {
	if len(events) == 0 {
		return nil
	}
	groups := make(map[factflow.ChannelSelectID]channelSelectResultGroup)
	for _, event := range events {
		selectID := event.SelectID()
		if selectID == "" {
			continue
		}
		group := groups[selectID]
		switch event.Kind() {
		case factflow.ChannelSelectSelect:
			group.resultIndex = event.Index()
			group.hasResult = true
		case factflow.ChannelSelectReceive:
			if _, ok := event.PayloadValue(); ok {
				group.cases = append(group.cases, event)
			} else if _, ok := event.CasePath(); ok {
				group.cases = append(group.cases, event)
			}
		}
		groups[selectID] = group
	}
	for selectID, group := range groups {
		sort.SliceStable(group.cases, func(i, j int) bool {
			return group.cases[i].Index() < group.cases[j].Index()
		})
		groups[selectID] = group
	}
	return groups
}

func channelSelectResultValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	selectID factflow.ChannelSelectID,
	cases []factflow.ChannelSelect,
) (product.Value, bool) {
	reg := ctx.Registry
	if reg == nil || len(cases) == 0 {
		return product.Value{}, false
	}
	resultCases := make([]channelselect.ResultCase, 0, len(cases))
	for _, event := range cases {
		payloadType, ok := channelSelectEventPayloadType(ctx, resolver, out, event)
		if !ok {
			continue
		}
		resultCases = append(resultCases, channelselect.ResultCase{
			Index:   event.Index(),
			Payload: payloadType,
		})
	}
	if len(resultCases) == 0 {
		return product.Value{}, false
	}
	resultType, ok := channelselect.ResultValueType(string(selectID), resultCases)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, resultType), resultType), true
}

func channelSelectEventPayloadType(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	event factflow.ChannelSelect,
) (typ.Type, bool) {
	if payloadValue, ok := event.PayloadValue(); ok {
		return valueWitnessType(ctx.Registry, payloadValue)
	}
	casePath, ok := event.CasePath()
	if !ok {
		return nil, false
	}
	return channelSelectCasePathPayloadType(ctx, resolver, out, casePath)
}

func channelSelectCasePathPayloadType(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	casePath pathdom.Path,
) (typ.Type, bool) {
	resolved, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, casePath)
	if !ok {
		return nil, false
	}
	channelType, ok := valueWitnessType(ctx.Registry, resolved.value)
	if !ok {
		return nil, false
	}
	return channelselect.ChannelPayloadType(channelType)
}

func valueWitnessType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	witness := product.Get(reg, value, typewitness.Key)
	if witness.IsTop() || witness.IsBottom() {
		return nil, false
	}
	return witness.Type()
}
