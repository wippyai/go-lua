package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// applyCallParamExposures eager-widens each argument object the callee exposes
// through a wider mutable view. Each exposure declares that the callee aliases the
// argument (or a member sub-path of it), at the wider mutable contract carried by
// Contract, into a slot the callee returns, stores into another argument, or
// retains in a captured sink; a write through that wider view can launder a wider
// value back into the argument object, so a later narrow read of the argument is
// no longer trustworthy. The rebased exposures enter the same factor-native N6
// transaction used by terminal concrete and formal execution.
func applyCallParamExposures(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	widen CovariantWiden,
	out state.State,
	paramBindings []pathdom.Path,
	exposures []callpayload.CallParamExposure,
) state.State {
	if widen == nil || len(exposures) == 0 {
		return out
	}
	transaction := CovariantExposureTransaction{point: ctx.Point}
	for _, exposure := range exposures {
		argPath, ok := exposure.Source.Substitute(paramBindings)
		if !ok || argPath.Symbol == 0 {
			continue
		}
		transaction.steps = append(transaction.steps, CovariantExposureStep{exposure: factflow.NewCovariantExposure(argPath, exposure.Contract, exposure.Kind)})
	}
	if transaction.Len() == 0 {
		return out
	}
	result := ApplyConcreteCovariantExposureTransaction(ConcreteCovariantExposureRequest{
		Context: ctx, Resolver: resolver, CovariantWiden: widen, Transaction: transaction, Input: out, Output: out,
	})
	if result.Err != nil || result.Canceled {
		return out
	}
	return result.Output
}

type resolvedCallParamLengthFloor struct {
	Path  pathdom.Path
	Floor int64
}

func resolveCallParamLengthFloors(
	resolver *visibility.Resolver,
	point cfg.Point,
	in state.State,
	paramBindings []pathdom.Path,
	facts []callpayload.CallParamLengthFloor,
) []resolvedCallParamLengthFloor {
	if len(facts) == 0 {
		return nil
	}
	out := make([]resolvedCallParamLengthFloor, 0, len(facts))
	for _, fact := range facts {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		floor := fact.Floor
		// Current param length floors are lowered from positive LengthChange
		// effects, so Floor is the minimum delta contributed by the call.
		// Resolve it against the incoming floor before mutation invalidations
		// clear stale descendants, then publish the post-call lower bound.
		if existing, ok := readCallParamLengthFloor(resolver, point, in, targetPath); ok {
			floor += existing
		}
		out = append(out, resolvedCallParamLengthFloor{Path: targetPath, Floor: floor})
	}
	return out
}

func readCallParamLengthFloor(
	resolver *visibility.Resolver,
	point cfg.Point,
	in state.State,
	targetPath pathdom.Path,
) (int64, bool) {
	if resolver == nil || targetPath.Symbol == 0 {
		return 0, false
	}
	pathKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	if !ok {
		return 0, false
	}
	return in.ReadLenFloor(resolver.KeySpace(), pathKey)
}

func applyCallParamLengthFloor(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	floor int64,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || floor <= 0 {
		return out
	}
	pathKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	if !ok {
		return out
	}
	return out.WriteLenFloor(resolver.KeySpace(), pathKey, floor)
}

func applyCallParamCondition(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	site factflow.CallSiteView,
	condition callpayload.CallParamCondition,
) state.State {
	arg, ok := site.ArgumentSourceAt(condition.ParamIndex)
	if !ok {
		return out
	}
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return out
	}
	expressionCondition, ok := facts.ExpressionCondition(arg.ExprRef)
	if !ok {
		return out
	}
	selectedFacts := expressionCondition.FactsForValue(condition.Value)
	return ApplyExpressionConditionFacts(ctx.Registry, resolver, projectPath, ctx.Point, out, selectedFacts)
}

func applyCallParamPathRelation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	bindings []pathdom.Path,
	relation callpayload.CallParamPathRelation,
) state.State {
	switch relation.Kind {
	case callpayload.CallPathRelationEqual:
		left, ok := relation.Left.Substitute(bindings)
		if !ok {
			return out
		}
		right, ok := relation.Right.Substitute(bindings)
		if !ok || left.Equal(right) {
			return out
		}
		next, _, err := applyPathEqualityFactorState(
			state.RegisteredProductDomain(ctx.Registry), nil, resolver, ctx.Point, out, left, right,
		)
		if err == nil {
			return next
		}
		return out
	default:
		return out
	}
}
