package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

func (s State) ReadDynamicIndexFact(reg *axis.Registry, key dynamicindex.Key) dynamicindex.Fact {
	return s.dynamicIndex.read(reg, key)
}

func (s State) WriteDynamicIndexFact(reg *axis.Registry, key dynamicindex.Key, fact dynamicindex.Fact) State {
	if key.Table == "" {
		return s
	}
	if s.dynamicIndex.top {
		panic("state: cannot finite-write dynamic index fact into top dynamic-index lane")
	}
	domain := dynamicindex.Domain(reg)
	if domain.Equal(fact, domain.Bottom()) {
		facts, changed := s.dynamicIndex.without(key)
		if !changed {
			return s
		}
		out := s.reachable()
		out.dynamicIndex = facts
		return out
	}
	if domain.Equal(s.dynamicIndex.read(reg, key), fact) {
		return s
	}
	out := s.reachable()
	out.dynamicIndex = s.dynamicIndex.with(key, fact)
	return out
}
