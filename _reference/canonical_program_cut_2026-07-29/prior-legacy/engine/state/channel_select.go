package state

import "github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"

func (s State) AddChannelSelectFact(fact channelselectfact.Fact) State {
	if fact.Select == "" || !s.laneEnabled(laneChannelSelectBit) {
		return s
	}
	out := s.reachable()
	out.channelSelect = out.channelSelect.Add(fact)
	return out
}

func (s State) HasChannelSelectFact(fact channelselectfact.Fact) bool {
	if !s.laneEnabled(laneChannelSelectBit) {
		return false
	}
	return s.channelSelect.Has(fact)
}
