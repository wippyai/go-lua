package sourcevalue

import (
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// NumFloorForSource derives the proven numeric floor for a value source.
func NumFloorForSource(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
) (int64, bool) {
	return numFloorForSource(reg, resolver, point, facts, in, source, nil)
}

// NumCeilForSource derives the proven numeric ceiling for a value source.
func NumCeilForSource(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
) (int64, bool) {
	return numCeilForSource(reg, resolver, point, facts, in, source, nil)
}

func numFloorForSource(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	if floor, ok := exactIntegerSource(reg, facts, source); ok {
		return floor, true
	}
	if floor, ok := pathSourceNumFloor(resolver, point, in, source); ok {
		return floor, true
	}
	if resolver == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return 0, false
	}
	if p, ok := facts.ExpressionPathRef(source.ExprRef); ok {
		pathKey, keyOK := visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
		if keyOK {
			if floor, ok := in.ReadNumFloor(resolver.KeySpace(), pathKey); ok {
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

func numCeilForSource(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	if ceil, ok := exactIntegerSource(reg, facts, source); ok {
		return ceil, true
	}
	if ceil, ok := pathSourceNumCeil(resolver, point, in, source); ok {
		return ceil, true
	}
	if resolver == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return 0, false
	}
	if p, ok := facts.ExpressionPathRef(source.ExprRef); ok {
		pathKey, keyOK := visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
		if keyOK {
			if ceil, ok := in.ReadNumCeil(resolver.KeySpace(), pathKey); ok {
				return ceil, true
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
	return numCeilForOperation(reg, resolver, point, facts, in, op, active)
}

func pathSourceNumFloor(
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	in state.State,
	source factflow.ValueSource,
) (int64, bool) {
	if resolver == nil || source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		return 0, false
	}
	if stateKey, ok := pathaddr.StateKeyFromPathKey(source.PathKey); ok {
		return in.ReadNumFloor(resolver.KeySpace(), stateKey)
	}
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
		p := pathdom.Path{Symbol: sym, Segments: segments}
		stateKey, keyOK := visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
		if !keyOK {
			return 0, false
		}
		return in.ReadNumFloor(resolver.KeySpace(), stateKey)
	}
	return 0, false
}

func pathSourceNumCeil(
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	in state.State,
	source factflow.ValueSource,
) (int64, bool) {
	if resolver == nil || source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		return 0, false
	}
	if stateKey, ok := pathaddr.StateKeyFromPathKey(source.PathKey); ok {
		return in.ReadNumCeil(resolver.KeySpace(), stateKey)
	}
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
		p := pathdom.Path{Symbol: sym, Segments: segments}
		stateKey, keyOK := visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
		if !keyOK {
			return 0, false
		}
		return in.ReadNumCeil(resolver.KeySpace(), stateKey)
	}
	return 0, false
}

func numFloorForOperation(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
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

func numCeilForOperation(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
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
		if ceil, ok := numCeilPlusConstant(reg, resolver, point, facts, in, op.Left(), op.Right(), active); ok {
			return ceil, true
		}
		return numCeilPlusConstant(reg, resolver, point, facts, in, op.Right(), op.Left(), active)
	case "-":
		c, ok := exactIntegerSource(reg, facts, op.Right())
		if !ok {
			return 0, false
		}
		left, ok := numCeilForSource(reg, resolver, point, facts, in, op.Left(), active)
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
	resolver visibility.PathKeyResolver,
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

func numCeilPlusConstant(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
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
	ceil, ok := numCeilForSource(reg, resolver, point, facts, in, source, active)
	if !ok {
		return 0, false
	}
	return checkedAddInt64(ceil, c)
}

func exactIntegerSource(reg *axis.Registry, facts factflow.Facts, source factflow.ValueSource) (int64, bool) {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return 0, false
		}
		value, ok := facts.ExpressionValue(source.ExprRef)
		if !ok {
			return 0, false
		}
		return typevalue.IntegerLiteralValue(reg, value)
	case factflow.ValueSourceLiteral:
		if source.LiteralKind == factflow.ValueSourceLiteralInteger {
			return source.Int, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func checkedAddInt64(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}
