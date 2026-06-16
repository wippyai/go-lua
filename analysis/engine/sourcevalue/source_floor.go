package sourcevalue

import (
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// PathKeyResolver resolves a path to the visibility-scoped key used by state
// lower-bound tracking.
type PathKeyResolver interface {
	KeyAt(point cfg.Point, p pathdom.Path) pathdom.PathKey
}

// NumFloorForSource derives the proven numeric floor for a value source.
func NumFloorForSource(
	reg *axis.Registry,
	resolver PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
) (int64, bool) {
	return numFloorForSource(reg, resolver, point, facts, in, source, nil)
}

func numFloorForSource(
	reg *axis.Registry,
	resolver PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	if resolver == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return 0, false
	}
	if value, ok := facts.ExpressionValue(source.ExprRef); ok {
		if floor, ok := typevalue.IntegerLiteralValue(reg, value); ok {
			return floor, true
		}
	}
	if p, ok := facts.ExpressionPath(source.ExprRef); ok {
		pathKey := numFloorKeyAt(resolver, point, p)
		if pathKey != "" {
			if floor, ok := in.ReadNumFloor(pathKey); ok {
				return floor, true
			}
		}
	}
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok {
		return 0, false
	}
	if active[source.ExprRef] {
		return 0, false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[source.ExprRef] = true
	defer delete(active, source.ExprRef)
	return numFloorForOperation(reg, resolver, point, facts, in, op, active)
}

func numFloorKeyAt(resolver PathKeyResolver, point cfg.Point, p pathdom.Path) pathdom.PathKey {
	if p.Symbol == 0 {
		return ""
	}
	if len(p.Segments) == 0 {
		return p.Key()
	}
	if resolver == nil {
		return ""
	}
	return resolver.KeyAt(point, p)
}

func numFloorForOperation(
	reg *axis.Registry,
	resolver PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	op factflow.ExpressionOperation,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	if op.Kind() != factflow.ExpressionOperationBinary {
		return 0, false
	}
	switch op.Op() {
	case "+":
		if floor, ok := numFloorPlusConstant(reg, resolver, point, facts, in, op.Left(), op.Right(), active); ok {
			return floor, true
		}
		return numFloorPlusConstant(reg, resolver, point, facts, in, op.Right(), op.Left(), active)
	case "-":
		c, ok := exactIntegerSource(reg, facts, op.Right())
		if !ok {
			return 0, false
		}
		left, ok := numFloorForSource(reg, resolver, point, facts, in, op.Left(), active)
		if !ok {
			return 0, false
		}
		return checkedAddInt64(left, -c)
	default:
		return 0, false
	}
}

func numFloorPlusConstant(
	reg *axis.Registry,
	resolver PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
	constant factflow.ValueSource,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	c, ok := exactIntegerSource(reg, facts, constant)
	if !ok {
		return 0, false
	}
	floor, ok := numFloorForSource(reg, resolver, point, facts, in, source, active)
	if !ok {
		return 0, false
	}
	return checkedAddInt64(floor, c)
}

func exactIntegerSource(reg *axis.Registry, facts factflow.Facts, source factflow.ValueSource) (int64, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return 0, false
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		return 0, false
	}
	return typevalue.IntegerLiteralValue(reg, value)
}

func checkedAddInt64(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}
