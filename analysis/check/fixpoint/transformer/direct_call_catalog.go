package transformer

import (
	"fmt"
	"slices"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DirectCallBoundary is the exact callee namespace order needed to bind
// RootCapture and RootGlobal terms from the caller relation. Parameters are
// already ordered by the call site's value-source list.
type DirectCallBoundary struct {
	Captures []symbol.ID
	Globals  []symbol.ID
}

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
	targets    []DirectCallTarget
	present    []bool
	boundaries []DirectCallBoundary
}

// NewDirectCallCatalog snapshots exact point identities. Duplicate identity is
// impossible because input is keyed by CFG point; zero cells and out-of-range
// points fail the entire catalog closed.
func NewDirectCallCatalog(pointCount int, input map[cfg.Point]DirectCallTarget) (DirectCallCatalog, error) {
	return NewDirectCallCatalogWithBoundaries(pointCount, input, nil)
}

// NewDirectCallCatalogWithBoundaries snapshots route identities and their
// ordered non-parameter lexical boundary symbols as one immutable catalog.
func NewDirectCallCatalogWithBoundaries(pointCount int, input map[cfg.Point]DirectCallTarget, boundaries map[cfg.Point]DirectCallBoundary) (DirectCallCatalog, error) {
	if pointCount < 0 {
		return DirectCallCatalog{}, fmt.Errorf("transformer: negative direct-call point count")
	}
	out := DirectCallCatalog{
		targets:    make([]DirectCallTarget, pointCount),
		present:    make([]bool, pointCount),
		boundaries: make([]DirectCallBoundary, pointCount),
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
		boundary := boundaries[point]
		// A nil sidecar is the legacy catalog contract: it identifies routes but
		// makes no claim that non-parameter boundary order is available. Preserve
		// that distinction so older activation/fallback policy remains unchanged.
		// The evaluated total catalog always supplies a non-nil map and therefore
		// receives strict width validation.
		if boundaries != nil && (len(boundary.Captures) != int(target.Shape.Captures) || len(boundary.Globals) != int(target.Shape.Globals)) {
			return DirectCallCatalog{}, fmt.Errorf("transformer: direct-call point %d boundary width differs from target shape", point)
		}
		out.boundaries[point] = DirectCallBoundary{
			Captures: append([]symbol.ID(nil), boundary.Captures...),
			Globals:  append([]symbol.ID(nil), boundary.Globals...),
		}
	}
	return out, nil
}

// Boundary returns an ownership copy of the target's non-parameter namespace.
func (c DirectCallCatalog) Boundary(point cfg.Point) (DirectCallBoundary, bool) {
	if uint64(point) >= uint64(len(c.boundaries)) || uint64(point) >= uint64(len(c.present)) || !c.present[point] {
		return DirectCallBoundary{}, false
	}
	boundary := c.boundaries[point]
	return DirectCallBoundary{Captures: append([]symbol.ID(nil), boundary.Captures...), Globals: append([]symbol.ID(nil), boundary.Globals...)}, true
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
	if len(c.targets) != len(other.targets) || len(c.present) != len(other.present) || len(c.boundaries) != len(other.boundaries) {
		return false
	}
	for point := range c.targets {
		if c.present[point] != other.present[point] {
			return false
		}
		if c.present[point] && c.targets[point] != other.targets[point] {
			return false
		}
		if c.present[point] && (!slices.Equal(c.boundaries[point].Captures, other.boundaries[point].Captures) || !slices.Equal(c.boundaries[point].Globals, other.boundaries[point].Globals)) {
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
