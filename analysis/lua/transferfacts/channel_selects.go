package transferfacts

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/channelruntime"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (l *lowerer) channelSelects(point cfg.Point, result *semantics.Result) []factflow.ChannelSelect {
	fact, ok := result.ChannelSelect(point)
	if !ok || !fact.ResultTarget.HasPath || fact.ResultTarget.Path.IsEmpty() {
		return nil
	}
	return l.channelSelectEvents(point, fact)
}

func (l *lowerer) channelSelectEvents(point cfg.Point, fact semantics.ChannelSelectFact) []factflow.ChannelSelect {
	if !fact.ResultTarget.HasPath || fact.ResultTarget.Path.IsEmpty() {
		return nil
	}
	selectID := factflow.ChannelSelectID("lua.channel_select@" + strconv.Itoa(int(point)))
	events := []factflow.ChannelSelect{
		factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID:      selectID,
			Kind:          factflow.ChannelSelectSelect,
			ResultPath:    fact.ResultTarget.Path,
			HasResultPath: true,
			Index:         fact.ResultTarget.ResultIndex,
			HasDefault:    fact.HasDefault,
		}),
	}
	for i, c := range fact.Cases {
		if !c.HasChannelPath || c.ChannelPath.IsEmpty() {
			continue
		}
		payloadValue, hasPayloadValue := l.channelSelectPayloadValue(c.ChannelPath)
		events = append(events,
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID:    selectID,
				Kind:        factflow.ChannelSelectCase,
				CasePath:    c.ChannelPath,
				HasCasePath: true,
				Index:       i,
			}),
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID:        selectID,
				Kind:            factflow.ChannelSelectReceive,
				ResultPath:      fact.ResultTarget.Path,
				HasResultPath:   true,
				CasePath:        c.ChannelPath,
				HasCasePath:     true,
				PayloadValue:    payloadValue,
				HasPayloadValue: hasPayloadValue,
				Index:           i,
			}),
		)
	}
	return events
}

func (l *lowerer) channelSelectPayloadValue(channelPath pathdom.Path) (product.Value, bool) {
	if l == nil || l.registry == nil {
		return product.Value{}, false
	}
	channelType, ok := l.channelSelectPathType(channelPath)
	if !ok {
		return product.Value{}, false
	}
	payloadType, ok := channelPayloadType(channelType)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.WithWitness(l.registry, typevalue.FromType(l.registry, payloadType), payloadType), true
}

func (l *lowerer) channelSelectPathType(p pathdom.Path) (typ.Type, bool) {
	if l == nil || p.Symbol == 0 {
		return nil, false
	}
	current, ok := l.symbolTypes[p.Symbol]
	if !ok {
		return nil, false
	}
	return typeaccess.ProjectSegments(current, p.Segments)
}

func channelPayloadType(channelType typ.Type) (typ.Type, bool) {
	return channelruntime.ChannelPayloadType(channelType)
}
