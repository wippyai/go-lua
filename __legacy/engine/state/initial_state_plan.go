package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// InitialCoordinate is a body-local semantic-program point. The State package
// deliberately does not depend on a language CFG representation; the body
// authority binds these ordinals to one exact prepared graph identity.
type InitialCoordinate uint32

// InitialStateSeed is one explicitly seeded CFG coordinate. The value is the
// raw caller-owned seed: reachability and State-domain normalization belong to
// the equation that admits it.
type InitialStateSeed struct {
	point InitialCoordinate
	value State
}

// NewInitialStateSeed detaches one point seed from construction scratch.
func NewInitialStateSeed(point InitialCoordinate, value State) InitialStateSeed {
	return InitialStateSeed{point: point, value: value.Snapshot()}
}

// InitialStatePlan is the finite, immutable replacement for transfer's
// callback-shaped InitialState input. It is bound to one stable lexical body
// and one exact prepared CFG. Every invocation of that body receives the same
// sparse equation constants; no solver may call back into configuration while
// iterating.
//
// A prepared empty plan is distinct from the zero value. Seeds are stored in
// the prepared graph's RPO order so construction map order cannot affect the
// frozen program.
type InitialStatePlan struct {
	owner      lexicalidentity.StableLexicalBodyID
	graphID    uint64
	pointCount int
	seeds      []InitialStateSeed
	prepared   bool
}

// NewInitialStatePlan validates, canonically orders and detaches seeds for one
// exact lexical body/graph pair. orderedPoints is the prepared body's complete
// solver order; graphID is its process-local immutable identity.
func NewInitialStatePlan(owner lexicalidentity.StableLexicalBodyID, graphID uint64, pointCount int, orderedPoints []InitialCoordinate, seeds []InitialStateSeed) (InitialStatePlan, error) {
	if owner == (lexicalidentity.StableLexicalBodyID{}) || graphID == 0 || pointCount <= 0 || len(orderedPoints) == 0 {
		return InitialStatePlan{}, fmt.Errorf("state: initial-state plan requires an owned lexical CFG")
	}
	order := make(map[InitialCoordinate]int, len(orderedPoints))
	for index, point := range orderedPoints {
		if _, duplicate := order[point]; duplicate {
			return InitialStatePlan{}, fmt.Errorf("state: initial-state plan graph repeats point %d", point)
		}
		order[point] = index
	}
	owned := append([]InitialStateSeed(nil), seeds...)
	seen := make(map[InitialCoordinate]struct{}, len(owned))
	for index := range owned {
		seed := &owned[index]
		if _, present := order[seed.point]; !present {
			return InitialStatePlan{}, fmt.Errorf("state: initial-state seed point %d is outside the prepared CFG", seed.point)
		}
		if _, duplicate := seen[seed.point]; duplicate {
			return InitialStatePlan{}, fmt.Errorf("state: initial-state seed point %d is duplicated", seed.point)
		}
		seen[seed.point] = struct{}{}
		seed.value = seed.value.Snapshot()
	}
	sort.Slice(owned, func(i, j int) bool { return order[owned[i].point] < order[owned[j].point] })
	return InitialStatePlan{owner: owner, graphID: graphID, pointCount: pointCount, seeds: owned, prepared: true}, nil
}

// Clone returns a detached immutable plan.
func (p InitialStatePlan) Clone() InitialStatePlan {
	if !p.prepared {
		return InitialStatePlan{}
	}
	p.seeds = append([]InitialStateSeed(nil), p.seeds...)
	return p
}

// Valid reports whether this value was constructed as an owned plan.
func (p InitialStatePlan) Valid() bool {
	return p.prepared && p.owner != (lexicalidentity.StableLexicalBodyID{}) && p.graphID != 0 && p.pointCount > 0
}

// ValidFor verifies the exact prepared lexical body and CFG authority.
func (p InitialStatePlan) ValidFor(owner lexicalidentity.StableLexicalBodyID, graphID uint64, pointCount int) bool {
	return p.Valid() && p.owner == owner && p.graphID == graphID && p.pointCount == pointCount
}

// Len returns the number of explicit point seeds.
func (p InitialStatePlan) Len() int { return len(p.seeds) }

// Empty reports whether this prepared plan has no explicit point seed.
func (p InitialStatePlan) Empty() bool { return len(p.seeds) == 0 }

// Seed returns the canonically ordered seed at index.
func (p InitialStatePlan) Seed(index int) (InitialCoordinate, State, bool) {
	if !p.Valid() || index < 0 || index >= len(p.seeds) {
		return 0, State{}, false
	}
	seed := p.seeds[index]
	return seed.point, seed.value.Snapshot(), true
}

// At returns the explicit seed for point. Absence means the coordinate starts
// from its ordinary equation bottom (or from the separately supplied entry).
func (p InitialStatePlan) At(point InitialCoordinate) (State, bool) {
	if !p.Valid() {
		return State{}, false
	}
	for _, seed := range p.seeds {
		if seed.point == point {
			return seed.value.Snapshot(), true
		}
	}
	return State{}, false
}
