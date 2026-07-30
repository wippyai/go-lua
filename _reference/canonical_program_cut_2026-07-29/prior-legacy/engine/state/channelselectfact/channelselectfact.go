// Package channelselectfact owns state-lane semantics for channel-select facts.
package channelselectfact

import (
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type ID string

type Kind uint8

const (
	FactSelect Kind = iota + 1
	FactReceive
	FactCase
)

type Fact struct {
	Select     ID
	Kind       Kind
	Result     pathaddr.StateKey
	Case       pathaddr.StateKey
	Index      int
	HasDefault bool
	Payload    product.Value
	HasPayload bool
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
	factDomain := lift.MustSet[Fact]()
	return lattice.Lattice[Lane]{
		Bottom: Bottom,
		Top:    Top,
		Equal: func(a, b Lane) bool {
			return factDomain.Equal(factLane(a), factLane(b))
		},
		Same: func(a, b Lane) bool {
			return factDomain.Same(factLane(a), factLane(b))
		},
		LessOrEq: func(a, b Lane) bool {
			return factDomain.LessOrEq(factLane(a), factLane(b))
		},
		Join: func(a, b Lane) Lane {
			return laneFromFactLane(factDomain.Join(factLane(a), factLane(b)))
		},
		Meet: func(a, b Lane) Lane {
			return laneFromFactLane(factDomain.Meet(factLane(a), factLane(b)))
		},
		Widen: func(prev, next Lane) Lane {
			return laneFromFactLane(factDomain.Widen(factLane(prev), factLane(next)))
		},
	}
}

func factLane(l Lane) lift.MustSetLane[Fact] {
	if l.bottom {
		return lift.MustSetBottom[Fact]()
	}
	return lift.MustSetValues(l.facts)
}

func laneFromFactLane(l lift.MustSetLane[Fact]) Lane {
	return Lane{
		bottom: l.Bottom(),
		// MustSetLane values are persistent and immutable once published.
		facts: l.Values(),
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
	if !l.bottom {
		if _, ok := l.facts[fact]; ok {
			return l
		}
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

// ForEachFact visits the finite must facts without allocating or imposing a
// presentation order. Returning false stops traversal. Bottom and Top have no
// finite facts to visit.
func (l Lane) ForEachFact(visit func(Fact) bool) bool {
	if visit == nil || l.bottom {
		return true
	}
	for fact := range l.facts {
		if !visit(fact) {
			return false
		}
	}
	return true
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
	if a.Index != b.Index {
		return a.Index < b.Index
	}
	return !a.HasDefault && b.HasDefault
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
