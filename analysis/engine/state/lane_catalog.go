package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// LaneCatalog is the registration boundary for State product-lattice axes.
// The State lattice container folds built lane operations; it does not know
// which semantic lanes exist or which subset a caller selected.
type LaneCatalog struct {
	factories []stateLaneFactory
}

// DefaultLaneCatalog returns the standard set of State lanes.
func DefaultLaneCatalog() LaneCatalog {
	return defaultDomainLaneCatalog
}

// LaneSet returns the ordered lane IDs in this catalog.
func (c LaneCatalog) LaneSet() LaneSet {
	out := make(LaneSet, 0, len(c.factories))
	for _, factory := range c.factories {
		out = append(out, factory.id)
	}
	return out
}

// Domain builds a State lattice with every lane in this catalog enabled.
func (c LaneCatalog) Domain(reg *axis.Registry) lattice.Lattice[State] {
	return domainFromLaneFactories(reg, c.factories)
}

// DomainWithLaneSet builds a State lattice from an exact ordered lane
// selection against this catalog.
func (c LaneCatalog) DomainWithLaneSet(reg *axis.Registry, lanes LaneSet) lattice.Lattice[State] {
	return domainFromLaneFactories(reg, c.selectFactories(lanes))
}

func (c LaneCatalog) selectFactories(lanes LaneSet) []stateLaneFactory {
	byID := make(map[LaneID]stateLaneFactory, len(c.factories))
	for _, factory := range c.factories {
		byID[factory.id] = factory
	}
	seen := make(map[LaneID]struct{}, len(lanes))
	out := make([]stateLaneFactory, 0, len(lanes))
	for _, id := range lanes {
		factory, ok := byID[id]
		if !ok {
			panic("state: unknown domain lane " + string(id))
		}
		if _, ok := seen[id]; ok {
			panic("state: duplicate domain lane " + string(id))
		}
		seen[id] = struct{}{}
		out = append(out, factory)
	}
	return out
}
