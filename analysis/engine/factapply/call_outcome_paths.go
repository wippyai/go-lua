package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func callOutcomeConcreteRootInvalidation(target pathdom.Path) bool {
	return !target.IsPlaceholder() && target.Symbol != 0 && len(target.Segments) == 0
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
		return visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
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
