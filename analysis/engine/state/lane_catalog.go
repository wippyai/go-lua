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
	specs []laneSpec
}

func newLaneCatalog(specs []laneSpec) LaneCatalog {
	out := make([]laneSpec, len(specs))
	copy(out, specs)
	for i := range out {
		if i >= 63 {
			panic("state: lane catalog supports at most 63 lanes")
		}
		out[i].bit = laneMask(1) << i
	}
	return LaneCatalog{specs: out}
}

// DefaultLaneCatalog returns the standard set of State lanes.
func DefaultLaneCatalog() LaneCatalog {
	return defaultLaneCatalog
}

// LaneSet returns the ordered lane IDs in this catalog.
func (c LaneCatalog) LaneSet() LaneSet {
	out := make([]LaneID, 0, len(c.specs))
	for _, spec := range c.specs {
		out = append(out, spec.id)
	}
	return LaneSet{ids: out}
}

// Domain builds a State lattice with every lane in this catalog enabled.
func (c LaneCatalog) Domain(reg *axis.Registry) lattice.Lattice[State] {
	return domainFromLaneSpecs(reg, c.specs, c.specs)
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
	specs, err := c.selectSpecs(lanes)
	if err != nil {
		return lattice.Lattice[State]{}, err
	}
	return domainFromLaneSpecs(reg, specs, c.specs), nil
}

// ValidateLaneSet checks that every selected lane exists in this catalog and
// that no lane is selected more than once.
func (c LaneCatalog) ValidateLaneSet(lanes LaneSet) error {
	_, err := c.selectSpecs(lanes)
	return err
}

func (c LaneCatalog) mustLaneBit(id LaneID) laneMask {
	for _, spec := range c.specs {
		if spec.id == id {
			return spec.bit
		}
	}
	panic(fmt.Sprintf("state: unknown lane %q", id))
}

type reachableLaneOp struct {
	bit           laneMask
	markReachable func(State) State
}

func (c LaneCatalog) reachableOps() []reachableLaneOp {
	out := make([]reachableLaneOp, 0, len(c.specs))
	for _, spec := range c.specs {
		if spec.markReachable != nil {
			out = append(out, reachableLaneOp{
				bit:           spec.bit,
				markReachable: spec.markReachable,
			})
		}
	}
	return out
}

func (c LaneCatalog) selectSpecs(lanes LaneSet) ([]laneSpec, error) {
	known := make(map[LaneID]struct{}, len(c.specs))
	for _, spec := range c.specs {
		known[spec.id] = struct{}{}
	}
	seen := make(map[LaneID]struct{}, lanes.Len())
	for _, id := range lanes.ids {
		if _, ok := known[id]; !ok {
			return nil, fmt.Errorf("state: unknown lane %q", id)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("state: duplicate lane %q", id)
		}
		seen[id] = struct{}{}
	}
	out := make([]laneSpec, 0, lanes.Len())
	for _, spec := range c.specs {
		if _, ok := seen[spec.id]; ok {
			out = append(out, spec)
		}
	}
	return out, nil
}
