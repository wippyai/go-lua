package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// DirectCallTarget is the complete syntax-free identity of one lexical call
// edge. Shape is pinned beside Cell so a stale cell compiled against another
// boundary cannot be substituted accidentally.
type DirectCallTarget struct {
	Cell  CellRef
	Shape Shape
}

// DirectCallCatalog is an immutable dense, point-indexed lexical call graph.
// Resolution belongs to preparation; relation compilation never dispatches by
// source name, line, or mutable type state.
type DirectCallCatalog struct {
	targets []DirectCallTarget
	present []bool
}

// NewDirectCallCatalog snapshots exact point identities. Duplicate identity is
// impossible because input is keyed by CFG point; zero cells and out-of-range
// points fail the entire catalog closed.
func NewDirectCallCatalog(pointCount int, input map[cfg.Point]DirectCallTarget) (DirectCallCatalog, error) {
	if pointCount < 0 {
		return DirectCallCatalog{}, fmt.Errorf("transformer: negative direct-call point count")
	}
	out := DirectCallCatalog{
		targets: make([]DirectCallTarget, pointCount),
		present: make([]bool, pointCount),
	}
	for point, target := range input {
		if uint64(point) >= uint64(pointCount) {
			return DirectCallCatalog{}, fmt.Errorf("transformer: direct-call point %d outside plan width %d", point, pointCount)
		}
		if target.Cell == (CellRef{}) {
			return DirectCallCatalog{}, fmt.Errorf("transformer: direct-call point %d has zero cell identity", point)
		}
		out.targets[point] = target
		out.present[point] = true
	}
	return out, nil
}

// Lookup returns the frozen target at point without allocating.
func (c DirectCallCatalog) Lookup(point cfg.Point) (DirectCallTarget, bool) {
	if uint64(point) >= uint64(len(c.targets)) || !c.present[point] {
		return DirectCallTarget{}, false
	}
	return c.targets[point], true
}

// PointCount is the dense preparation width.
func (c DirectCallCatalog) PointCount() int { return len(c.targets) }

// Equal reports exact dense route equality. Shape equality is part of every
// target comparison: redirecting a point to a same-shaped cell is still a
// different compiled equation.
func (c DirectCallCatalog) Equal(other DirectCallCatalog) bool {
	if len(c.targets) != len(other.targets) || len(c.present) != len(other.present) {
		return false
	}
	for point := range c.targets {
		if c.present[point] != other.present[point] {
			return false
		}
		if c.present[point] && c.targets[point] != other.targets[point] {
			return false
		}
	}
	return true
}

// Cells returns sorted unique dependencies for RelationCell validation.
func (c DirectCallCatalog) Cells() []CellRef {
	refs := make([]CellRef, 0)
	seen := make(map[CellRef]struct{})
	for point, present := range c.present {
		if !present {
			continue
		}
		ref := c.targets[point].Cell
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	sortCellRefs(refs)
	return refs
}
