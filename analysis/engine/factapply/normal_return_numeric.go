package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func applyNormalReturnNumFloors(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.NumFloors {
		out = applyNormalReturnNumFloor(ctx, out, fact)
	}
	return out
}

func applyNormalReturnNumFloor(
	ctx normalReturnApplyContext,
	out state.State,
	fact callboundary.NumFloorFact,
) state.State {
	targetPath, ok := ctx.substitute(fact.Path)
	if !ok || targetPath.Symbol == 0 {
		return out
	}
	pathKey, ok := visibility.AddressAt(ctx.resolver, ctx.point, targetPath).RootOrVisibleStateKey()
	if !ok {
		return out
	}
	return out.WriteNumFloor(ctx.resolver.KeySpace(), pathKey, fact.Floor)
}

func applyNormalReturnRelConstraints(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.RelConstraints {
		out = applyNormalReturnRelConstraint(ctx, out, fact)
	}
	return out
}

func applyNormalReturnRelConstraint(
	ctx normalReturnApplyContext,
	out state.State,
	fact callboundary.RelConstraintFact,
) state.State {
	aKey, ok := ctx.relationGraphKey(fact.A)
	if !ok {
		return out
	}
	cKey, ok := ctx.relationGraphKey(fact.C)
	if !ok {
		return out
	}
	var bKey state.RelOperand
	coB := fact.CoB
	if coB != 0 && !fact.B.Path.IsEmpty() {
		bKey, ok = ctx.relationGraphKey(fact.B)
		if !ok {
			return out
		}
	} else {
		coB = 0
	}
	return out.WriteScaledConstraint(fact.CoA, aKey, coB, bKey, cKey, fact.K)
}
