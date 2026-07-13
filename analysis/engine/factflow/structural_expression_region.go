package factflow

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// StructuralExpressionRegion is immutable, source-authored control-flow
// metadata for one short-circuit expression. The map key in Facts is the
// expression which owns the result. OwnedRHSPoints is the complete conditional
// region, including points with effects not understood by today's axes.
type StructuralExpressionRegion struct {
	branch         cfg.Point
	trueTarget     cfg.Point
	falseTarget    cfg.Point
	join           cfg.Point
	rhsOnTrue      bool
	ownedRHSPoints []cfg.Point
}

// NewStructuralExpressionRegion validates and canonicalizes an exact region.
// Incomplete boundaries are rejected rather than published partially.
func NewStructuralExpressionRegion(
	branch, trueTarget, falseTarget, join cfg.Point,
	rhsOnTrue bool,
	ownedRHSPoints []cfg.Point,
) (StructuralExpressionRegion, bool) {
	if branch == trueTarget || branch == falseTarget || trueTarget == falseTarget || len(ownedRHSPoints) == 0 {
		return StructuralExpressionRegion{}, false
	}
	points := append([]cfg.Point(nil), ownedRHSPoints...)
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	out := points[:0]
	for _, point := range points {
		if point == branch || point == join {
			return StructuralExpressionRegion{}, false
		}
		if len(out) == 0 || out[len(out)-1] != point {
			out = append(out, point)
		}
	}
	rhsTarget := falseTarget
	bypassTarget := trueTarget
	if rhsOnTrue {
		rhsTarget = trueTarget
		bypassTarget = falseTarget
	}
	if bypassTarget != join {
		return StructuralExpressionRegion{}, false
	}
	index := sort.Search(len(out), func(i int) bool { return out[i] >= rhsTarget })
	if index == len(out) || out[index] != rhsTarget {
		return StructuralExpressionRegion{}, false
	}
	return StructuralExpressionRegion{
		branch: branch, trueTarget: trueTarget, falseTarget: falseTarget,
		join: join, rhsOnTrue: rhsOnTrue, ownedRHSPoints: out,
	}, true
}

func (r StructuralExpressionRegion) Branch() cfg.Point      { return r.branch }
func (r StructuralExpressionRegion) TrueTarget() cfg.Point  { return r.trueTarget }
func (r StructuralExpressionRegion) FalseTarget() cfg.Point { return r.falseTarget }
func (r StructuralExpressionRegion) Join() cfg.Point        { return r.join }
func (r StructuralExpressionRegion) RHSOnTrue() bool        { return r.rhsOnTrue }

// OwnedRHSPoints returns a defensive copy in canonical point order.
func (r StructuralExpressionRegion) OwnedRHSPoints() []cfg.Point {
	return append([]cfg.Point(nil), r.ownedRHSPoints...)
}

func (r StructuralExpressionRegion) copy() StructuralExpressionRegion {
	r.ownedRHSPoints = append([]cfg.Point(nil), r.ownedRHSPoints...)
	return r
}

func (r StructuralExpressionRegion) valid() bool {
	_, ok := NewStructuralExpressionRegion(r.branch, r.trueTarget, r.falseTarget, r.join, r.rhsOnTrue, r.ownedRHSPoints)
	return ok
}
