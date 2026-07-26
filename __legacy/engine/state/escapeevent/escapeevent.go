// Package escapeevent owns state-lane semantics for escape/ownership events.
package escapeevent

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	placementvocab "github.com/wippyai/go-lua/analysis/domain/placement/vocab"
)

// Kind is the legacy state-lane compatibility name for the canonical
// cross-boundary escape vocabulary.
type Kind = placementvocab.Escape

const (
	KindNone   Kind = placementvocab.None
	KindBorrow Kind = placementvocab.Borrow
	KindRetain Kind = placementvocab.Retain
	KindStore  Kind = placementvocab.Store
	KindSend   Kind = placementvocab.Send
	KindExport Kind = placementvocab.Export
	KindOpaque Kind = placementvocab.Opaque
)

// Fact records a must escape/seal event for a state target. Recursive means the
// event applies to the whole target subtree.
type Fact struct {
	Target    pathaddr.StateKey
	Kind      Kind
	Recursive bool
}

// Lane is a must-set of typed escape events. Compression and strength
// normalization stay in the summary layer; the point state preserves the facts
// observed along all incoming paths.
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

func (l Lane) Reachable() Lane {
	l.bottom = false
	return l
}

func (l Lane) Add(fact Fact) (Lane, bool) {
	if fact.Target == "" || fact.Kind == KindNone {
		return l, false
	}
	if !l.bottom {
		if _, ok := l.facts[fact]; ok {
			return l, false
		}
	}
	facts := cloneSet(l.facts)
	if facts == nil {
		facts = make(map[Fact]struct{}, 1)
	}
	facts[fact] = struct{}{}
	l = l.Reachable()
	l.facts = facts
	return l, true
}

func (l Lane) Has(fact Fact) bool {
	if l.bottom || fact.Target == "" || fact.Kind == KindNone {
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
	if a.Target != b.Target {
		return a.Target < b.Target
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return !a.Recursive && b.Recursive
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
