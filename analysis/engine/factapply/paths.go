package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func appendStaticMemberSuffix(ks *keyspace.KeySpace, base keyspace.Key, suffix []segment.Segment) (keyspace.Key, bool) {
	out := base
	for _, seg := range suffix {
		next, ok := ks.AppendSegment(out, seg)
		if !ok {
			return keyspace.Key{}, false
		}
		out = next
	}
	return out, true
}

func applyPathDescendantInvalidation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.PathDescendantInvalidation,
	clearStructuralWitness bool,
) state.State {
	if resolver == nil {
		return out
	}
	containerPath := fact.ContainerPathRef()
	if containerPath.IsEmpty() {
		return out
	}
	container, err := FreezeResolvedPathAddress(resolver, ctx.Point, containerPath)
	if err != nil {
		return out
	}
	resolved := &resolvedPathDescendantInvalidationData{
		Container:              container,
		ClearStructuralWitness: clearStructuralWitness,
	}
	if precise, ok := freezePreciseDynamicDescendantAddress(ctx, resolver, sources, read, in, out, fact); ok {
		resolved.Precise = precise
		resolved.HasPrecise = true
	}
	invalidated, ok := ApplyResolvedPathDescendantInvalidation(ctx.Registry, resolver.KeySpace(), out, ResolvedPathDescendantInvalidation{data: resolved})
	if !ok {
		return out
	}
	_ = facts
	return invalidated
}

func freezePreciseDynamicDescendantAddress(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.PathDescendantInvalidation,
) (ResolvedPathAddress, bool) {
	tablePath, keySource, suffix, ok := fact.DynamicTargetRef()
	if !ok || resolver == nil || sources == nil {
		return ResolvedPathAddress{}, false
	}
	keyValue, ok := sources.ValueOfSource(ctx.Point, keySource, in, readWithCurrentPointState(ctx.Point, read, out))
	if !ok {
		return ResolvedPathAddress{}, false
	}
	member, ok := staticSegmentFromValue(ctx.Registry, keyValue)
	if !ok {
		return ResolvedPathAddress{}, false
	}
	targetPath := tablePath.Append(member).AppendSegments(suffix)
	address, err := FreezeResolvedPathAddress(resolver, ctx.Point, targetPath)
	return address, err == nil
}

func staticSegmentFromValue(reg *axis.Registry, value product.Value) (segment.Segment, bool) {
	name, ok := staticStringKey(reg, value)
	if ok {
		return segment.Segment{Kind: segment.SegmentField, Name: name}, true
	}
	return segment.Segment{}, false
}
