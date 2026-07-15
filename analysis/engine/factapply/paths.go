package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyPathAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.PathAssignment,
) (state.State, bool) {
	if resolver == nil {
		return out, false
	}
	targetPath := fact.TargetPathRef()
	if len(targetPath.Segments) == 0 {
		return out, false
	}
	source := fact.Source()
	value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, out))
	if !ok {
		return out, false
	}
	invalidated := out
	if withOrigins, ok := invalidateRootOriginsForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath, false); ok {
		invalidated = withOrigins
	}
	if staticMemberAssignmentPreservesHeapSlot(ctx.Registry, facts, ctx.Point, targetPath, value) {
		if withHeap, ok := invalidateHeapStaticMemberDescendantsAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath); ok {
			invalidated = withHeap
		}
	} else {
		if withHeap, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath); ok {
			invalidated = withHeap
		}
	}
	invalidated, ok = invalidatePathSubtreeAt(invalidated, resolver, ctx.Point, targetPath)
	if !ok {
		return out, false
	}
	written, ok := writePathAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath, value)
	if !ok {
		return out, false
	}
	written = copySourcePathStaticMemberDescendantsAt(resolver, facts, ctx.Point, written, targetPath, source)
	written = addPathEqualityProofFromSource(resolver, facts, ctx.Point, written, targetPath, source)
	written = applyUserLatticeAssignment(ctx, resolver, facts, in, written, targetPath, source)
	return written, true
}

func staticMemberAssignmentPreservesHeapSlot(
	reg *axis.Registry,
	facts factflow.Facts,
	point cfg.Point,
	targetPath pathdom.Path,
	value product.Value,
) bool {
	write, ok := facts.PathStaticMemberWrite(point)
	return ok &&
		write.TargetPathRef().Equal(targetPath) &&
		!typevalue.HasOnlyNilType(reg, value)
}

type pathStaticMemberCopy struct {
	key   keyspace.Key
	value product.Value
}

func copySourcePathStaticMemberDescendantsAt(
	resolver *visibility.Resolver,
	facts factflow.Facts,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 {
		return out
	}
	if covariantExposureSuppressesPathProof(facts, resolver, point, source) {
		return out
	}
	sourcePath, ok := sourcePathFromValueSource(resolver, facts, source)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return out
	}
	sourceKey, ok := visibility.AddressAt(resolver, point, sourcePath).VisibleLocalKeyspaceKey()
	if !ok {
		return out
	}
	targetKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleLocalKeyspaceKey()
	if !ok {
		return out
	}
	ks := resolver.KeySpace()
	var copies []pathStaticMemberCopy
	out.ForEachPathStaticMember(func(memberKey keyspace.Key, memberValue product.Value) bool {
		suffix, ok := ks.ExactRemainderAfterPrefix(memberKey, sourceKey)
		if !ok || len(suffix) == 0 {
			return true
		}
		targetMemberKey, ok := appendStaticMemberSuffix(ks, targetKey, suffix)
		if !ok {
			return true
		}
		copies = append(copies, pathStaticMemberCopy{key: targetMemberKey, value: memberValue})
		return true
	})
	for _, copy := range copies {
		out = out.WriteLocalPathStaticMember(copy.key, copy.value)
	}
	return out
}

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
