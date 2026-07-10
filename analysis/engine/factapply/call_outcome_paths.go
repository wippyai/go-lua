package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func callOutcomeConcreteRootInvalidation(target pathdom.Path) bool {
	return !target.IsPlaceholder() && target.Symbol != 0 && len(target.Segments) == 0
}

// invalidateMutatedFieldSlot drops the caller's stored value for a field slot a
// callee wrote through. A field-level path invalidation (segments > 0) records
// that the callee assigned the slot to a value of its own, wider parameter field
// type, so the slot's confined caller value (a fresh literal's heap static member
// or path-key refinement) is no longer trustworthy. Descendant invalidation alone
// preserves the slot's own value, which would launder the covariant write-through;
// clearing the slot itself makes the later read fall back to structural
// projection, matching how an opaque parameter argument already reflects the
// mutation. Root-targeted invalidations (segments == 0) keep their container value
// and are handled by the descendant pass above.
func invalidateMutatedFieldSlot(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	if len(targetPath.Segments) == 0 {
		return out
	}
	if withHeap, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, out, resolver, ctx.Point, targetPath); ok {
		out = withHeap
	}
	if cleared, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath); ok {
		out = cleared
	}
	return out
}

func writePathInvalidationMarker(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	preserveStructuralWitness bool,
) state.State {
	targetKey, ok := factKeyspaceKeyAt(resolver, point, targetPath)
	if !ok && substitutedRootPath(targetPath) {
		targetKey, ok = visibility.AddressAt(resolver, point, targetPath).RootOrVisibleKeyspaceKey()
	}
	if ok {
		site := callboundary.PathInvalidationEffectSite()
		if preserveStructuralWitness {
			site = callboundary.PathStructuralPreservingInvalidationEffectSite()
		}
		return out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   site,
			Kind:   effectdelta.Mutation,
		}, effectdelta.Top())
	}
	return out
}

func callOutcomePathKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	path pathdom.Path,
) (pathdom.PathKey, bool) {
	targetPath, ok := boundaryPaths.Substitute(path)
	if !ok {
		return "", false
	}
	targetKey := factPathKeyAt(resolver, point, targetPath)
	if targetKey == "" && substitutedRootPath(targetPath) {
		targetKey, _ = visibility.AddressAt(resolver, point, targetPath).RootOrVisiblePathKey()
	}
	if targetKey == "" {
		return "", false
	}
	return targetKey, true
}

func callOutcomeStateKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	path pathdom.Path,
) (pathaddr.StateKey, bool) {
	targetPath, ok := boundaryPaths.Substitute(path)
	if !ok {
		return "", false
	}
	if callboundary.IsConcreteSymbolPath(path) || substitutedRootPath(targetPath) {
		return visibility.AddressAt(resolver, point, targetPath).RootOrVisibleStateKey()
	}
	return factStateKeyAt(resolver, point, targetPath)
}

func callOutcomeVisibleStateKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	path pathdom.Path,
) (pathaddr.StateKey, bool) {
	targetPath, ok := boundaryPaths.Substitute(path)
	if !ok {
		return "", false
	}
	if substitutedRootPath(targetPath) {
		return visibility.AddressAt(resolver, point, targetPath).RootOrVisibleStateKey()
	}
	return factStateKeyAt(resolver, point, targetPath)
}

func callOutcomeKeyspaceKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	boundaryPaths callboundary.PathBindings,
	path pathdom.Path,
) (keyspace.Key, bool) {
	targetPath, ok := boundaryPaths.Substitute(path)
	if !ok {
		return keyspace.Key{}, false
	}
	if callboundary.IsConcreteSymbolPath(path) || substitutedRootPath(targetPath) {
		return visibility.AddressAt(resolver, point, targetPath).RootOrVisibleKeyspaceKey()
	}
	return factKeyspaceKeyAt(resolver, point, targetPath)
}

func substitutedRootPath(path pathdom.Path) bool {
	return path.Root == "" && path.Symbol != 0 && len(path.Segments) == 0
}
