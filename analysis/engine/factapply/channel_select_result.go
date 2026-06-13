package factapply

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/channelselect"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type channelSelectResultGroup struct {
	resultIndex int
	hasResult   bool
	cases       []factflow.ChannelSelect
}

func applyChannelSelectResult(
	ctx transfer.NodeContext,
	out state.State,
	events []factflow.ChannelSelect,
) state.State {
	groups := channelSelectResultGroups(events)
	for selectID, group := range groups {
		if !group.hasResult || group.resultIndex < 0 || len(group.cases) == 0 {
			continue
		}
		resultValue, ok := channelSelectResultValue(ctx.Registry, selectID, group.cases)
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
	reg *axis.Registry,
	selectID factflow.ChannelSelectID,
	cases []factflow.ChannelSelect,
) (product.Value, bool) {
	if reg == nil || len(cases) == 0 {
		return product.Value{}, false
	}
	resultCases := make([]channelselect.ResultCase, 0, len(cases))
	for _, event := range cases {
		payloadValue, ok := event.PayloadValue()
		if !ok {
			continue
		}
		payloadType, ok := valueWitnessType(reg, payloadValue)
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

func valueWitnessType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	witness := product.Get(reg, value, typewitness.Key)
	if witness.IsTop() || witness.IsBottom() {
		return nil, false
	}
	return witness.Type()
}
