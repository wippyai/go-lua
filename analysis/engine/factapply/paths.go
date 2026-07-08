package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
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
	if withHeap, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath); ok {
		invalidated = withHeap
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
	out = applyPreciseDynamicDescendantInvalidation(ctx, resolver, facts, sources, read, in, out, fact)
	preservedStatic := preserveExactStaticMemberWitness(resolver, ctx.Point, out, containerPath, !clearStructuralWitness)
	invalidated := out
	if withOrigins, ok := invalidateRootOriginsForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, containerPath, true); ok {
		invalidated = withOrigins
	}
	if clearStructuralWitness {
		if withWitness, ok := invalidateRootStructuralWitnessForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, containerPath); ok {
			invalidated = withWitness
		}
	}
	if withHeap, ok := invalidateHeapStaticMemberDescendantsAt(ctx.Registry, invalidated, resolver, ctx.Point, containerPath); ok {
		invalidated = withHeap
	}
	invalidateDescendants := invalidatePathDescendantsAt
	if !clearStructuralWitness {
		invalidateDescendants = invalidatePathDescendantsPreservingDynamicValueKeyMembershipsAt
	}
	if withDescendants, ok := invalidateDescendants(invalidated, resolver, ctx.Point, containerPath); ok {
		invalidated = withDescendants
	}
	// A write into the container can change its length, so drop difference
	// relations over that length (and the container value) regardless of whether
	// the container carried any tracked descendant refinements.
	if containerKey, ok := visibility.AddressAt(resolver, ctx.Point, containerPath).RootOrVisibleStateKey(); ok {
		invalidated = invalidated.ClearDiffConstraintsFor(containerKey)
		if clearStructuralWitness {
			if localKey, ok := resolver.KeySpace().InternStateKey(containerKey); ok {
				invalidated = invalidated.ClearDynamicIndexValueKeyMembershipsForContainer(localKey)
			}
			invalidated = invalidated.ClearKeyMembershipsForPath(containerKey)
		}
	}
	invalidated = restoreExactStaticMemberWitness(resolver, invalidated, preservedStatic)
	return invalidated
}

func applyPreciseDynamicDescendantInvalidation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.PathDescendantInvalidation,
) state.State {
	tablePath, keySource, suffix, ok := fact.DynamicTargetRef()
	if !ok || resolver == nil || sources == nil {
		return out
	}
	keyValue, ok := sources.ValueOfSource(ctx.Point, keySource, in, readWithCurrentPointState(ctx.Point, read, out))
	if !ok {
		return out
	}
	member, ok := staticSegmentFromValue(ctx.Registry, keyValue)
	if !ok {
		return out
	}
	targetPath := tablePath.Append(member).AppendSegments(suffix)
	invalidated := out
	if withOrigins, ok := invalidateRootOriginsForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath, true); ok {
		invalidated = withOrigins
	}
	if withWitness, ok := invalidateRootStructuralWitnessForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath); ok {
		invalidated = withWitness
	}
	if withHeap, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath); ok {
		invalidated = withHeap
	}
	if withPath, ok := invalidatePathSubtreeAt(invalidated, resolver, ctx.Point, targetPath); ok {
		invalidated = withPath
	}
	_ = facts
	return invalidated
}

func staticSegmentFromValue(reg *axis.Registry, value product.Value) (segment.Segment, bool) {
	name, ok := staticStringKey(reg, value)
	if ok {
		return segment.Segment{Kind: segment.SegmentField, Name: name}, true
	}
	return segment.Segment{}, false
}

type exactStaticMemberWitness struct {
	key   pathdom.PathKey
	value product.Value
}

func preserveExactStaticMemberWitness(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	target pathdom.Path,
	enabled bool,
) []exactStaticMemberWitness {
	if !enabled || resolver == nil {
		return nil
	}
	targetKey := factPathKeyAt(resolver, point, target)
	if targetKey == "" {
		return nil
	}
	value, ok := out.ReadPathStaticMember(resolver.KeySpace(), targetKey)
	if !ok {
		return nil
	}
	return []exactStaticMemberWitness{{key: targetKey, value: value}}
}

func restoreExactStaticMemberWitness(resolver *visibility.Resolver, out state.State, witnesses []exactStaticMemberWitness) state.State {
	if resolver == nil || len(witnesses) == 0 {
		return out
	}
	ks := resolver.KeySpace()
	for _, witness := range witnesses {
		out = out.WritePathStaticMember(ks, witness.key, witness.value)
	}
	return out
}
