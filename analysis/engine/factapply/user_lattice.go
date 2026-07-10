package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyUserLatticeAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || targetPath.IsEmpty() || targetPath.Symbol == 0 {
		return out
	}
	targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetPath).RootOrVisibleStateKey()
	if !ok {
		return out
	}
	sourceKey, ok := userLatticeSourceStateKey(resolver, ctx.Point, facts, source)
	if !ok {
		return out.ClearUserElements(ctx.Registry, resolver.KeySpace(), targetKey)
	}
	return out.PropagateUserAssignmentFrom(ctx.Registry, resolver.KeySpace(), targetKey, in, sourceKey)
}

func userLatticeSourceStateKey(
	resolver *visibility.Resolver,
	point cfg.Point,
	facts factflow.Facts,
	source factflow.ValueSource,
) (pathaddr.StateKey, bool) {
	if source.Kind == factflow.ValueSourcePath && source.PathKey != "" {
		if stateKey, ok := pathaddr.StateKeyFromPathKey(source.PathKey); ok {
			return stateKey, true
		}
	}
	sourcePath, ok := sourcePathFromValueSource(resolver, facts, source)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return "", false
	}
	return visibility.AddressAt(resolver, point, sourcePath).RootOrVisibleStateKey()
}
