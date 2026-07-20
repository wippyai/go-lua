package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type normalReturnApplyContext struct {
	node          transfer.NodeContext
	typeValues    *typevalue.Cache
	resolver      *visibility.Resolver
	projectPath   PathTypeProjector
	point         cfg.Point
	boundaryPaths callboundary.PathBindings
	normalFacts   callboundary.NormalReturnFacts
}

func (ctx normalReturnApplyContext) substitute(path pathdom.Path) (pathdom.Path, bool) {
	return ctx.boundaryPaths.Substitute(path)
}

func (ctx normalReturnApplyContext) pathKey(path pathdom.Path) (pathdom.PathKey, bool) {
	return callOutcomePathKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) keyspaceKey(path pathdom.Path) (keyspace.Key, bool) {
	return callOutcomeKeyspaceKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) stateKey(path pathdom.Path) (pathaddr.StateKey, bool) {
	return callOutcomeStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) visibleStateKey(path pathdom.Path) (pathaddr.StateKey, bool) {
	return callOutcomeVisibleStateKeyAt(ctx.resolver, ctx.point, ctx.boundaryPaths, path)
}

func (ctx normalReturnApplyContext) relationGraphKey(operand callboundary.RelOperand) (state.RelOperand, bool) {
	targetPath, ok := ctx.substitute(operand.Path)
	if !ok || targetPath.Symbol == 0 {
		return state.RelOperand{}, false
	}
	return relationGraphKeyAt(ctx.resolver, ctx.point, targetPath, operand.IsLength)
}

func boundaryRootBoundToDescendant(factPath, targetPath pathdom.Path) bool {
	if len(factPath.Segments) != 0 || len(targetPath.Segments) == 0 {
		return false
	}
	if factPath.IsPlaceholder() {
		return true
	}
	_, ok := callboundary.ReturnSlotIndex(factPath)
	return ok
}
