package factapply

import (
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
	object, ok = object.WithStaticMember(resolver.KeySpace(), suffix, value)
	if !ok {
		return out
	}
	return out.WriteHeapTableObject(ctx.Registry, id, object)
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
		return markReachableHeapValuePlacement(ctx.Registry, out, value, ownerPlacement, map[identity.ID]struct{}{})
	default:
		return out
	}
}
