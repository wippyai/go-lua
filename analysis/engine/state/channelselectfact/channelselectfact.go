// Package channelselectfact owns state-lane semantics for channel-select facts.
package channelselectfact

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type ID string

type Kind uint8

const (
	FactSelect Kind = iota + 1
	FactReceive
	FactCase
)

type Fact struct {
	Select ID
	Kind   Kind
	Result pathdom.PathKey
	Case   pathdom.PathKey
	Index  int
}

type Lane struct {
	bottom bool
	facts  map[Fact]struct{}
}

type Snapshot struct {
	Bottom bool
	Top    bool
	Facts  []Fact
}

func Bottom() Lane {
	return Lane{bottom: true}
}

func Top() Lane {
	return Lane{}
}

func Domain() lattice.Lattice[Lane] {
	return lattice.Lattice[Lane]{
		Bottom: Bottom,
		Top:    Top,
		Equal: func(a, b Lane) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return finiteSetEqual(a.facts, b.facts)
		},
		LessOrEq: func(a, b Lane) bool {
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return finiteMustSetLessOrEq(a.facts, b.facts)
			}
		},
		Join: func(a, b Lane) Lane {
			if a.bottom {
				return b.Clone()
			}
			if b.bottom {
				return a.Clone()
			}
			return Lane{facts: finiteSetIntersection(a.facts, b.facts)}
		},
		Widen: func(prev, next Lane) Lane {
			if prev.bottom {
				return next.Clone()
			}
			if next.bottom {
				return prev.Clone()
			}
			return Lane{facts: finiteSetIntersection(prev.facts, next.facts)}
		},
	}
}

func (l Lane) Clone() Lane {
	return Lane{
		bottom: l.bottom,
		facts:  cloneSet(l.facts),
	}
}

func (l Lane) Reachable() Lane {
	l.bottom = false
	return l
}

func (l Lane) Add(fact Fact) Lane {
	if fact.Select == "" {
		return l
	}
	facts := cloneSet(l.facts)
	if facts == nil {
		facts = make(map[Fact]struct{}, 1)
	}
	facts[fact] = struct{}{}
	l = l.Reachable()
	l.facts = facts
	return l
}

func (l Lane) Has(fact Fact) bool {
	if l.bottom {
		return false
	}
	_, ok := l.facts[fact]
	return ok
}

func (l Lane) Snapshot() Snapshot {
	if l.bottom {
		return Snapshot{Bottom: true}
	}
	facts := factsFromSet(l.facts)
	return Snapshot{
		Top:   len(facts) == 0,
		Facts: facts,
	}
}

func factsFromSet(in map[Fact]struct{}) []Fact {
	if len(in) == 0 {
		return nil
	}
	out := make([]Fact, 0, len(in))
	for fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		return factLess(out[i], out[j])
	})
	return out
}

func factLess(a, b Fact) bool {
	if a.Select != b.Select {
		return a.Select < b.Select
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Result != b.Result {
		return a.Result < b.Result
	}
	if a.Case != b.Case {
		return a.Case < b.Case
	}
	return a.Index < b.Index
}

func cloneSet(in map[Fact]struct{}) map[Fact]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[Fact]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func finiteSetEqual(a, b map[Fact]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for v := range a {
		if _, ok := b[v]; !ok {
			return false
		}
	}
	return true
}

func finiteMustSetLessOrEq(a, b map[Fact]struct{}) bool {
	for v := range b {
		if _, ok := a[v]; !ok {
			return false
		}
	}
	return true
}

func finiteSetIntersection(a, b map[Fact]struct{}) map[Fact]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[Fact]struct{})
	for v := range a {
		if _, ok := b[v]; ok {
			out[v] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
