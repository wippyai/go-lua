package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
)

type cfgSymbolSet struct {
	seen map[cfg.SymbolID]struct{}
}

func (s *cfgSymbolSet) Add(sym cfg.SymbolID) bool {
	if sym == 0 {
		return false
	}
	if s.seen == nil {
		s.seen = make(map[cfg.SymbolID]struct{})
	}
	if _, ok := s.seen[sym]; ok {
		return false
	}
	s.seen[sym] = struct{}{}
	return true
}

func (s cfgSymbolSet) Contains(sym cfg.SymbolID) bool {
	if sym == 0 || s.seen == nil {
		return false
	}
	_, ok := s.seen[sym]
	return ok
}

func (s cfgSymbolSet) Len() int {
	return len(s.seen)
}

type cfgSymbolList struct {
	set    cfgSymbolSet
	values []cfg.SymbolID
}

func (l *cfgSymbolList) Add(sym cfg.SymbolID) bool {
	if !l.set.Add(sym) {
		return false
	}
	l.values = append(l.values, sym)
	return true
}

func (l *cfgSymbolList) SortedValues() []cfg.SymbolID {
	if len(l.values) == 0 {
		return nil
	}
	out := append([]cfg.SymbolID(nil), l.values...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
