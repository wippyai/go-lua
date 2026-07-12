package relationcall

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Route is one preparation-owned lexical call edge. Cell and SummaryKey stay
// inseparable so a relation cannot be adapted under another function identity.
type Route struct {
	Point  cfg.Point
	Target Target
}

// Catalog is an immutable dense call-point routing table.
type Catalog struct {
	targets []Target
	present []bool
}

// NewCatalog rejects missing identities, out-of-range points, and ambiguous
// duplicate bindings atomically. Input order has no semantic effect.
func NewCatalog(pointCount int, routes []Route) (Catalog, error) {
	if pointCount < 0 {
		return Catalog{}, fmt.Errorf("relationcall: negative point count")
	}
	out := Catalog{targets: make([]Target, pointCount), present: make([]bool, pointCount)}
	for _, route := range routes {
		if uint64(route.Point) >= uint64(pointCount) {
			return Catalog{}, fmt.Errorf("relationcall: point %d outside plan width %d", route.Point, pointCount)
		}
		if out.present[route.Point] {
			return Catalog{}, fmt.Errorf("relationcall: ambiguous target at point %d", route.Point)
		}
		if route.Target.Cell == (transformer.CellRef{}) {
			return Catalog{}, fmt.Errorf("relationcall: point %d has zero cell identity", route.Point)
		}
		if route.Target.SummaryKey == (summary.SummaryKey{}) {
			return Catalog{}, fmt.Errorf("relationcall: point %d has zero summary identity", route.Point)
		}
		out.targets[route.Point] = route.Target
		out.present[route.Point] = true
	}
	return out, nil
}

// Lookup returns the frozen target without allocating.
func (c Catalog) Lookup(point cfg.Point) (Target, bool) {
	if uint64(point) >= uint64(len(c.targets)) || !c.present[point] {
		return Target{}, false
	}
	return c.targets[point], true
}

func (c Catalog) PointCount() int { return len(c.targets) }
