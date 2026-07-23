package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func returnSourceHasDeclaredContract(facts factflow.Facts, source factflow.ValueSource) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	refinement, ok := facts.ExpressionRefinement(source.ExprRef)
	return ok && refinement.Mode() == factflow.ExpressionRefinementDeclaredContract
}

func returnSourceValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if _, ok := facts.ExpressionRefinement(source.ExprRef); ok {
			if value, ok := sources.ValueOfSource(ctx.Point, source, out, readWithCurrentPointState(ctx.Point, read, out)); ok {
				return value, true
			}
		}
		if sourcePath, ok := facts.ExpressionPathRef(source.ExprRef); ok {
			if pathValue, ok := resolvePathValueAtCached(typeValues, ctx.Registry, resolver, ctx.Point, out, sourcePath, projectPath); ok {
				if cached, cachedOK := facts.ExpressionValue(source.ExprRef); cachedOK {
					if preserved, preserveOK := sourcevalue.PreservePathBackedGradualContract(ctx.Registry, typeValues, cached, pathValue.value); preserveOK {
						return preserved, true
					}
				}
				return pathValue.value, true
			}
		}
	}
	return sources.ValueOfSource(ctx.Point, source, out, readWithCurrentPointState(ctx.Point, read, out))
}
