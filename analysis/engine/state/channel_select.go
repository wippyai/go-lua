package state

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

type ChannelSelectID string

type ChannelSelectFactKind uint8

const (
	ChannelSelectFactSelect ChannelSelectFactKind = iota + 1
	ChannelSelectFactReceive
	ChannelSelectFactCase
)

type ChannelSelectFact struct {
	Select ChannelSelectID
	Kind   ChannelSelectFactKind
	Result pathdom.PathKey
	Case   pathdom.PathKey
	Index  int
}

func (s State) AddChannelSelectFact(fact ChannelSelectFact) State {
	if fact.Select == "" {
		return s
	}
	facts := cloneChannelSelectSet(s.channelSelect)
	if facts == nil {
		facts = make(map[ChannelSelectFact]struct{}, 1)
	}
	facts[fact] = struct{}{}
	out := s.reachable()
	out.channelSelect = facts
	return out
}

func (s State) HasChannelSelectFact(fact ChannelSelectFact) bool {
	if s.channelSelectBottom {
		return false
	}
	_, ok := s.channelSelect[fact]
	return ok
}

func cloneChannelSelectSet(in map[ChannelSelectFact]struct{}) map[ChannelSelectFact]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ChannelSelectFact]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}
