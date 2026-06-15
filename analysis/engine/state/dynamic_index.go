package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

func (s State) ReadDynamicIndexFact(reg *axis.Registry, key dynamicindex.Key) dynamicindex.Fact {
	if key.Table == "" {
		return dynamicindex.Bottom(reg)
	}
	if s.dynamicIndexTop {
		return dynamicindex.Top()
	}
	if fact, ok := s.dynamicIndex[key]; ok {
		return fact
	}
	return dynamicindex.Bottom(reg)
}

func (s State) WriteDynamicIndexFact(reg *axis.Registry, key dynamicindex.Key, fact dynamicindex.Fact) State {
	if key.Table == "" {
		return s
	}
	if s.dynamicIndexTop {
		panic("state: cannot finite-write dynamic index fact into top dynamic-index lane")
	}
	domain := dynamicindex.Domain(reg)
	if domain.Equal(fact, domain.Bottom()) {
		facts, changed := dynamicindex.DeleteEntry(s.dynamicIndex, key)
		if !changed {
			return s
		}
		out := s.reachable()
		out.dynamicIndex = facts
		return out
	}
	facts := dynamicindex.CloneMap(s.dynamicIndex)
	if facts == nil {
		facts = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
	}
	facts[key] = fact
	out := s.reachable()
	out.dynamicIndex = facts
	return out
}
