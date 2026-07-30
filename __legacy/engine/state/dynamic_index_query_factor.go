package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// DynamicIndexTableEvidence is the exact finite cone for one table. Top is
// explicit and carries no facts.
type DynamicIndexTableEvidence struct {
	Top   bool
	Facts []dynamicindex.Fact
}

// ObserveDynamicIndexTableFactor selects one registered table cone without
// exposing or scanning unrelated axes.
func (d ProductDomain) ObserveDynamicIndexTableFactor(factor LaneFactor, table keyspace.Key) (DynamicIndexTableEvidence, error) {
	if table.Kind == keyspace.KindInvalid {
		return DynamicIndexTableEvidence{}, ErrInvalidLaneFactor
	}
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return DynamicIndexTableEvidence{}, err
	}
	lane, ok := d.ProductLane(LaneDynamicIndex)
	if !ok || runtime.lane != lane {
		return DynamicIndexTableEvidence{}, fmt.Errorf("%w: dynamic-index query has no registered owner", ErrInvalidLaneFactor)
	}
	values := typedLaneFactorValue[dynamicIndexLane](factor.payload)
	if values.isTop() {
		return DynamicIndexTableEvidence{Top: true}, nil
	}
	type entry struct {
		site dynamicindex.Site
		fact dynamicindex.Fact
	}
	entries := make([]entry, 0)
	for key, fact := range values.values {
		if key.Table == table {
			entries = append(entries, entry{site: key.Site, fact: fact})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].site < entries[j].site })
	out := DynamicIndexTableEvidence{Facts: make([]dynamicindex.Fact, len(entries))}
	for index := range entries {
		out.Facts[index] = entries[index].fact
	}
	return out, nil
}
