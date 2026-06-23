package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// LaneCatalog is the registration boundary for State product-lattice axes.
// The State container folds registered lane operations and reachability
// transitions; it does not need a second hand-maintained list of semantic lanes.
type LaneCatalog struct {
	factories []stateLaneFactory
}

// DefaultLaneCatalog returns the standard set of State lanes.
func DefaultLaneCatalog() LaneCatalog {
	return defaultDomainLaneCatalog
}

// LaneSet returns the ordered lane IDs in this catalog.
func (c LaneCatalog) LaneSet() LaneSet {
	out := make([]LaneID, 0, len(c.factories))
	for _, factory := range c.factories {
		out = append(out, factory.id)
	}
	return LaneSet{ids: out}
}

// Domain builds a State lattice with every lane in this catalog enabled.
func (c LaneCatalog) Domain(reg *axis.Registry) lattice.Lattice[State] {
	return domainFromLaneFactories(reg, c.factories)
}

// DomainWithLaneSet builds a State lattice from an exact ordered lane
// selection against this catalog.
func (c LaneCatalog) DomainWithLaneSet(reg *axis.Registry, lanes LaneSet) lattice.Lattice[State] {
	domain, err := c.TryDomainWithLaneSet(reg, lanes)
	if err != nil {
		panic(err)
	}
	return domain
}

// TryDomainWithLaneSet builds a State lattice from an exact ordered lane
// selection against this catalog, returning configuration errors instead of
// panicking.
func (c LaneCatalog) TryDomainWithLaneSet(reg *axis.Registry, lanes LaneSet) (lattice.Lattice[State], error) {
	factories, err := c.selectFactories(lanes)
	if err != nil {
		return lattice.Lattice[State]{}, err
	}
	return domainFromLaneFactories(reg, factories), nil
}

// ValidateLaneSet checks that every selected lane exists in this catalog and
// that no lane is selected more than once.
func (c LaneCatalog) ValidateLaneSet(lanes LaneSet) error {
	_, err := c.selectFactories(lanes)
	return err
}

func (c LaneCatalog) reachableOps() []func(State) State {
	out := make([]func(State) State, 0, len(c.factories))
	for _, factory := range c.factories {
		if factory.markReachable != nil {
			out = append(out, factory.markReachable)
		}
	}
	return out
}

func (c LaneCatalog) selectFactories(lanes LaneSet) ([]stateLaneFactory, error) {
	byID := make(map[LaneID]stateLaneFactory, len(c.factories))
	for _, factory := range c.factories {
		byID[factory.id] = factory
	}
	seen := make(map[LaneID]struct{}, lanes.Len())
	out := make([]stateLaneFactory, 0, lanes.Len())
	for _, id := range lanes.ids {
		factory, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("state: unknown domain lane %q", id)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("state: duplicate domain lane %q", id)
		}
		seen[id] = struct{}{}
		out = append(out, factory)
	}
	return out, nil
}
