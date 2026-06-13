package state

import "github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"

type ChannelSelectID = channelselectfact.ID

type ChannelSelectFactKind = channelselectfact.Kind

const (
	ChannelSelectFactSelect  = channelselectfact.FactSelect
	ChannelSelectFactReceive = channelselectfact.FactReceive
	ChannelSelectFactCase    = channelselectfact.FactCase
)

type ChannelSelectFact = channelselectfact.Fact

func (s State) AddChannelSelectFact(fact ChannelSelectFact) State {
	if fact.Select == "" {
		return s
	}
	out := s.reachable()
	out.channelSelect = out.channelSelect.Add(fact)
	return out
}

func (s State) HasChannelSelectFact(fact ChannelSelectFact) bool {
	return s.channelSelect.Has(fact)
}
