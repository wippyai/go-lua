// Package paramuse owns the normalized evidence algebra for parameter-use
// observations. The API evidence slice is a wire format; producers and
// consumers should use Set so whole-use and demanded-field semantics are not
// reimplemented independently.
package paramuse

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
)

// Use records the surface demanded from one parameter symbol.
type Use struct {
	whole  bool
	fields map[fieldkey.Key]struct{}
}

func (u Use) Whole() bool {
	return u.whole
}

func (u Use) HasFields() bool {
	return len(u.fields) > 0
}

func (u Use) Observed() bool {
	return u.whole || len(u.fields) > 0
}

func (u Use) Fields() map[fieldkey.Key]struct{} {
	return u.fields
}

// Set is the canonical in-memory form for parameter-use evidence.
type Set struct {
	uses map[cfg.SymbolID]Use
}

// FromEvidence lowers API wire evidence into a normalized Set.
func FromEvidence(evidence []api.ParameterUseEvidence) Set {
	var set Set
	for _, ev := range evidence {
		if ev.Symbol == 0 {
			continue
		}
		if ev.Whole {
			set.MarkWhole(ev.Symbol)
		}
		for _, field := range ev.Fields {
			if key, ok := fieldkey.FromSegment(field); ok {
				set.Field(ev.Symbol, key)
			}
		}
	}
	return set
}

func (s *Set) MarkWhole(sym cfg.SymbolID) {
	if sym == 0 {
		return
	}
	use := s.use(sym)
	use.whole = true
	s.uses[sym] = use
}

func (s *Set) Field(sym cfg.SymbolID, key fieldkey.Key) {
	if sym == 0 {
		return
	}
	use := s.use(sym)
	if use.fields == nil {
		use.fields = make(map[fieldkey.Key]struct{}, 1)
	}
	use.fields[key] = struct{}{}
	s.uses[sym] = use
}

func (s *Set) Get(sym cfg.SymbolID) (Use, bool) {
	if sym == 0 || s.uses == nil {
		return Use{}, false
	}
	use, ok := s.uses[sym]
	return use, ok
}

func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.uses)
}

func (s *Set) IsEmpty() bool {
	return s == nil || len(s.uses) == 0
}

func (s *Set) use(sym cfg.SymbolID) Use {
	if s.uses == nil {
		s.uses = make(map[cfg.SymbolID]Use)
	}
	return s.uses[sym]
}

// Evidence exports the set as stable API wire evidence.
func (s *Set) Evidence() []api.ParameterUseEvidence {
	if s.IsEmpty() {
		return nil
	}
	syms := make([]int, 0, len(s.uses))
	for sym := range s.uses {
		syms = append(syms, int(sym))
	}
	sort.Ints(syms)

	out := make([]api.ParameterUseEvidence, 0, len(syms))
	for _, raw := range syms {
		sym := cfg.SymbolID(raw)
		use := s.uses[sym]
		ev := api.ParameterUseEvidence{Symbol: sym, Whole: use.whole}
		if len(use.fields) > 0 {
			ev.Fields = append(ev.Fields, fieldkey.Sorted(use.fields)...)
		}
		out = append(out, ev)
	}
	return out
}
