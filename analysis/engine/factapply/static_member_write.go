package factapply

import (
	"context"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func writeHeapTableStaticMember(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
	value product.Value,
) state.State {
	if resolver == nil || len(targetPath.Segments) == 0 {
		return out
	}
	ownerPath := targetPath.Clone()
	suffix := ownerPath.Segments[len(ownerPath.Segments)-1:]
	ownerPath.Segments = ownerPath.Segments[:len(ownerPath.Segments)-1]
	owner, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, ownerPath, nil)
	if !ok {
		return out
	}
	id, ok := product.Get(ctx.Registry, owner.value, identity.Key).ID()
	if !ok {
		return out
	}
	object := out.ReadHeapTableObject(ctx.Registry, id)
	if heapidentity.ObjectDomain(ctx.Registry).Equal(object, heapidentity.BottomObject(ctx.Registry)) {
		return out
	}
	object, ok = object.WithStaticMember(ctx.Registry, resolver.KeySpace(), suffix, value)
	if !ok {
		return out
	}
	domain := state.RegisteredProductDomain(ctx.Registry)
	plan, err := domain.PrepareObjectGraphReplacePlan(resolver.KeySpace(), []state.ObjectGraphMutation{{Identity: identity.ConcreteTerm(id), Object: object}})
	if err != nil {
		return out
	}
	written, err := domain.ApplyObjectGraphMutation(plan, out)
	if err != nil {
		return out
	}
	return written
}

func applyStoredStaticMemberPlacement(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
	value product.Value,
) state.State {
	if resolver == nil || len(targetPath.Segments) == 0 {
		return out
	}
	ownerPath := targetPath.Clone()
	ownerPath.Segments = ownerPath.Segments[:len(ownerPath.Segments)-1]
	owner, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, ownerPath, nil)
	if !ok {
		return out
	}
	ownerID, ok := product.Get(ctx.Registry, owner.value, identity.Key).ID()
	if !ok {
		return out
	}
	ownerPlacement := out.ReadPlacement(ownerID)
	switch ownerPlacement {
	case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		domain := state.RegisteredProductDomain(ctx.Registry)
		plan, err := domain.PreparePlacementReachabilityPlan(resolver.KeySpace(), []product.Value{value}, ownerPlacement)
		if err != nil {
			return out
		}
		applyContext := ctx.Context
		if applyContext == nil {
			applyContext = context.Background()
		}
		written, err := domain.ApplyPlacementReachability(applyContext, plan, out)
		if err != nil {
			return out
		}
		return written
	default:
		return out
	}
}
