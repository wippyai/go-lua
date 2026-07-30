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

// NumericAffinePlan is the frozen exact source-demand vocabulary shared by
// lower- and upper-bound transfer. The supported source algebra is affine by
// an integer constant: an exact literal, or one path-bound plus an offset.
// This is exactly the algebra previously interpreted separately by the floor
// and ceiling walkers, now represented once for concrete and factor execution.
type NumericAffinePlan struct {
	path   pathaddr.StateKey
	offset int64
	exact  bool
	sealed bool
}

// PlanNumericAffineSource compiles one source into its exact numeric-bound
// demand. Cyclic expression graphs and unsupported arithmetic have no bound;
// there is no depth cap and no approximation.
func PlanNumericAffineSource(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	source factflow.ValueSource,
) (NumericAffinePlan, bool) {
	return planNumericAffineSource(reg, resolver, point, facts, source, nil)
}

func planNumericAffineSource(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	source factflow.ValueSource,
	active map[factflow.ExprRef]bool,
) (NumericAffinePlan, bool) {
	if value, ok := exactIntegerSource(reg, facts, source); ok {
		return NumericAffinePlan{offset: value, exact: true, sealed: true}, true
	}
	if path, ok := numericSourceStateKey(resolver, point, facts, source); ok {
		return NumericAffinePlan{path: path, sealed: true}, true
	}
	if resolver == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return NumericAffinePlan{}, false
	}
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || active[source.ExprRef] {
		return NumericAffinePlan{}, false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[source.ExprRef] = true
	defer delete(active, source.ExprRef)

	var base factflow.ValueSource
	var delta int64
	switch op.Op() {
	case "+":
		if constant, exact := exactIntegerSource(reg, facts, op.Right()); exact {
			base, delta = op.Left(), constant
		} else if constant, exact := exactIntegerSource(reg, facts, op.Left()); exact {
			base, delta = op.Right(), constant
		} else {
			return NumericAffinePlan{}, false
		}
	case "-":
		constant, exact := exactIntegerSource(reg, facts, op.Right())
		if !exact || constant == math.MinInt64 {
			return NumericAffinePlan{}, false
		}
		base, delta = op.Left(), -constant
	default:
		return NumericAffinePlan{}, false
	}
	plan, ok := planNumericAffineSource(reg, resolver, point, facts, base, active)
	if !ok {
		return NumericAffinePlan{}, false
	}
	next, ok := checkedAddInt64(plan.offset, delta)
	if !ok {
		return NumericAffinePlan{}, false
	}
	plan.offset = next
	return plan, true
}

func numericSourceStateKey(
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	source factflow.ValueSource,
) (pathaddr.StateKey, bool) {
	if resolver == nil {
		return "", false
	}
	if source.Kind == factflow.ValueSourcePath && source.PathKey != "" {
		if stateKey, ok := pathaddr.StateKeyFromPathKey(source.PathKey); ok {
			return stateKey, true
		}
		if sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
			return visibility.AddressAt(resolver, point, pathdom.Path{Symbol: sym, Segments: segments}).RootOrVisibleStateKey()
		}
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if path, ok := facts.ExpressionPathRef(source.ExprRef); ok {
			return visibility.AddressAt(resolver, point, path).RootOrVisibleStateKey()
		}
	}
	return "", false
}

// Exact reports whether p is a constant and returns that constant.
func (p NumericAffinePlan) Exact() (int64, bool) {
	return p.offset, p.sealed && p.exact
}

// Source reports the sole path-bound input and additive offset.
func (p NumericAffinePlan) Source() (pathaddr.StateKey, int64, bool) {
	return p.path, p.offset, p.sealed && !p.exact && p.path != ""
}

// Evaluate applies p to one bound reader. The reader determines whether the
// same plan observes lower or upper facts; arithmetic and overflow semantics
// are therefore shared exactly.
func (p NumericAffinePlan) Evaluate(read func(pathaddr.StateKey) (int64, bool)) (int64, bool) {
	if !p.sealed {
		return 0, false
	}
	if p.exact {
		return p.offset, true
	}
	if read == nil || p.path == "" {
		return 0, false
	}
	value, ok := read(p.path)
	if !ok {
		return 0, false
	}
	return checkedAddInt64(value, p.offset)
}

// NumFloorForSource derives the proven numeric floor for a value source.
func NumFloorForSource(
	reg *axis.Registry,
	resolver visibility.PathKeyResolver,
	point cfg.Point,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
) (int64, bool) {
	plan, ok := PlanNumericAffineSource(reg, resolver, point, facts, source)
	if !ok {
		return 0, false
	}
	return plan.Evaluate(func(key pathaddr.StateKey) (int64, bool) {
		return in.ReadNumFloor(resolver.KeySpace(), key)
	})
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
	plan, ok := PlanNumericAffineSource(reg, resolver, point, facts, source)
	if !ok {
		return 0, false
	}
	return plan.Evaluate(func(key pathaddr.StateKey) (int64, bool) {
		return in.ReadNumCeil(resolver.KeySpace(), key)
	})
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
	}
	return 0, false
}

func checkedAddInt64(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}
