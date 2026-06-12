package transferfacts

import (
	"strconv"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
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
		}),
	}
	for i, c := range fact.Cases {
		if !c.HasChannelPath || c.ChannelPath.IsEmpty() {
			continue
		}
		events = append(events,
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID:    selectID,
				Kind:        factflow.ChannelSelectCase,
				CasePath:    c.ChannelPath,
				HasCasePath: true,
				Index:       i,
			}),
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID:      selectID,
				Kind:          factflow.ChannelSelectReceive,
				ResultPath:    fact.ResultTarget.Path,
				HasResultPath: true,
				CasePath:      c.ChannelPath,
				HasCasePath:   true,
				Index:         i,
			}),
		)
	}
	return events
}
