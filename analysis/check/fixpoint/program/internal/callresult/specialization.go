package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

// SummaryArgumentTypeProvider resolves call argument types using summary-backed
// function expression identities, then falls back to generic source-value
// projection. It is used by public signature lowering when imported generic
// calls receive local callbacks whose return types are known only after the
// program fixed point.
func SummaryArgumentTypeProvider(config ProviderConfig) func(transfer.NodeContext, factflow.ValueSource, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
	summaries := config.Summaries
	facts := config.Facts
	index := summaryIndexFromProviderConfig(config)
	functionKeys := index.functionKeys
	functionExpressionKeys := index.functionExpressionKeys
	functionTypes := index.functionTypes
	sources := config.Sources
	return func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
		rootDeclarations := rootDeclarationQueryProvider(facts, ctx.Graph)
		return callArgumentType(ctx, source, summaries, facts, rootDeclarations, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
	}
}

func specializeGenericSummary(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	got summary.Summary,
	fn *typ.Function,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
	typeValues *typevalue.Cache,
) summary.Summary {
	if ctx.Registry == nil || fn == nil || len(fn.TypeParams) == 0 || sources == nil {
		return got
	}
	args := callArgumentTypes(ctx, site, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
	if len(args) == 0 {
		return got
	}
	instantiated, violations := typecall.InstantiateGenericCall(fn, args)
	if len(violations) != 0 || instantiated == nil || instantiated == fn {
		return got
	}
	return specializeSummaryReturns(ctx.Registry, typeValues, got, fn.Returns, instantiated.Returns)
}

func summaryNeedsGenericInstantiation(ctx transfer.NodeContext, fn *typ.Function, sources sourcevalue.SourceValues) bool {
	return ctx.Registry != nil && fn != nil && len(fn.TypeParams) != 0 && sources != nil
}

func callArgumentTypes(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []typ.Type {
	if sources == nil {
		return nil
	}
	argCount := site.ArgumentSourceCount()
	if argCount == 0 {
		return nil
	}
	rootDeclarations := rootDeclarationQueryProvider(facts, ctx.Graph)
	var args []typ.Type
	for i := 0; i < argCount; i++ {
		source, ok := site.ArgumentSourceAt(i)
		if !ok {
			continue
		}
		t, ok := callArgumentType(ctx, source, summaries, facts, rootDeclarations, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
		if !ok {
			continue
		}
		if args == nil {
			args = make([]typ.Type, argCount)
		}
		args[i] = t
	}
	return args
}

func rootDeclarationQueryProvider(
	facts factflow.Facts,
	graph cfg.Graph,
) func() factquery.RootDeclarationQuery {
	var query factquery.RootDeclarationQuery
	var ready bool
	return func() factquery.RootDeclarationQuery {
		if !ready {
			query = factquery.NewRootDeclarationQuery(facts, graph)
			ready = true
		}
		return query
	}
}

func sourceValueAtArgument(
	reg *axis.Registry,
	point cfg.Point,
	source factflow.ValueSource,
	facts factflow.Facts,
	rootDeclarations func() factquery.RootDeclarationQuery,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	value, ok := valueOfSource(point, source, sources, in, read)
	if t, okType := inferenceTypeFromValue(reg, value); okType && UsableType(reg, value, t) {
		return value, true
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return value, ok
	}
	if value, ok := valueFromRootDeclarationSource(reg, point, source.ExprRef, facts, rootDeclarations, sources, in, read); ok {
		return value, true
	}
	return value, ok
}

func valueFromRootDeclarationSource(
	reg *axis.Registry,
	point cfg.Point,
	expr factflow.ExprRef,
	facts factflow.Facts,
	rootDeclarations func() factquery.RootDeclarationQuery,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if reg == nil {
		return product.Value{}, false
	}
	exprPath, ok := facts.ExpressionPathRef(expr)
	if !ok || exprPath.Symbol == 0 || len(exprPath.Segments) != 0 {
		return product.Value{}, false
	}
	if rootDeclarations == nil {
		return product.Value{}, false
	}
	decl, ok := rootDeclarations().DominatingRootDeclarationSource(point, exprPath.Symbol)
	if !ok {
		return product.Value{}, false
	}
	declState := in
	if read != nil {
		declState = read(decl.Point)
	}
	if decl.Symbol != 0 {
		v := declState.ReadSymbolValue(reg, decl.Symbol)
		if !product.Equal(reg, v, product.Bottom(reg)) {
			return v, true
		}
	}
	return valueOfSource(decl.Point, decl.Source, sources, declState, read)
}

func valueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}

func callArgumentType(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	summaries summary.Reader,
	facts factflow.Facts,
	rootDeclarations func() factquery.RootDeclarationQuery,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (typ.Type, bool) {
	if t, ok := functionExpressionTypeFromSummary(ctx, source, summaries, facts, functionKeys, functionExpressionKeys, functionTypes); ok {
		return t, true
	}
	if sources == nil {
		return nil, false
	}
	value, ok := sourceValueAtArgument(ctx.Registry, ctx.Point, source, facts, rootDeclarations, sources, in, read)
	if !ok {
		return nil, false
	}
	t, ok := inferenceTypeFromValue(ctx.Registry, value)
	if !ok || !UsableType(ctx.Registry, value, t) {
		return nil, false
	}
	return t, true
}

func UsableType(reg *axis.Registry, value product.Value, t typ.Type) bool {
	if reg == nil || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || refinement.ContainsFreeTypeParam(t) {
		return false
	}
	return proof.New(reg, nil).TypeEvidenceUsableForInference(value)
}

func functionExpressionTypeFromSummary(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
) (typ.Type, bool) {
	if !source.HasExpr || summaries == nil {
		return nil, false
	}
	key, ok := functionExpressionKeys[source.ExprRef]
	if !ok {
		functionSymbol, ok := facts.ExpressionFunction(source.ExprRef)
		if !ok || functionSymbol == 0 {
			return nil, false
		}
		key, ok = functionKeys[functionSymbol]
		if !ok {
			return nil, false
		}
	}
	fn := functionTypes[key]
	if fn == nil {
		return nil, false
	}
	sum, ok, _ := readProviderSummary(summaries, key)
	if !ok || len(sum.Returns) == 0 {
		return fn, true
	}
	out := functionTypeWithSummaryReturns(ctx.Registry, fn, sum.Returns)
	return out, true
}

func functionTypeWithSummaryReturns(reg *axis.Registry, fn *typ.Function, returns []product.Value) *typ.Function {
	if reg == nil || fn == nil || len(returns) == 0 {
		return fn
	}
	outReturns := make([]typ.Type, 0, len(returns))
	for _, ret := range returns {
		t, ok := typeFromValue(reg, ret)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || refinement.ContainsFreeTypeParam(t) {
			return fn
		}
		outReturns = append(outReturns, t)
	}
	if len(outReturns) == 0 {
		return fn
	}
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: fn.TypeParams,
		Params:     fn.Params,
		Variadic:   fn.Variadic,
		Returns:    outReturns,
	})
}

func typeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return typevalue.TypeOf(reg, value)
}

func inferenceTypeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	return proof.New(reg, nil).ValueTypeWithPresence(value)
}
