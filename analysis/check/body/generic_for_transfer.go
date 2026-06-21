package body

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func genericForNodeTransfer(
	base transfer.NodeTransfer,
	sem *semantics.Result,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	signatures signaturelookup.Source,
	signatureID *signatureIdentityResolver,
	typeResolver *typeresolve.Resolver,
	typeValues *typevalue.Cache,
	callOutcome callpayload.CallOutcomeProvider,
	ks *keyspace.KeySpace,
) transfer.NodeTransfer {
	expressionRefinements := facts.ExpressionRefinements()
	return func(ctx transfer.NodeContext, in state.State) state.State {
		out := in
		if base != nil {
			out = base(ctx, in)
		}
		if sem == nil || sources == nil || signatureID == nil {
			return out
		}
		fact, ok := sem.GenericFor(ctx.Point)
		if !ok || fact.Role != cfgfacts.GenericForRoleVariable || !fact.HasSymbols ||
			fact.VariableIndex < 0 || fact.VariableIndex >= len(fact.Symbols) {
			return out
		}
		value, ok := genericForVariableValue(ctx, typeValues, fact, facts, expressionRefinements, sources, signatures, signatureID, typeResolver, callOutcome, ks, in)
		if !ok {
			return out
		}
		target := fact.Symbols[fact.VariableIndex]
		if target == 0 {
			return out
		}
		return out.WriteValue(ctx.Registry, key.SymbolValue(target), value)
	}
}

func genericForVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	generic cfgfacts.GenericForFact,
	facts factflow.Facts,
	expressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement,
	sources sourcevalue.SourceValues,
	signatures signaturelookup.Source,
	signatureID *signatureIdentityResolver,
	typeResolver *typeresolve.Resolver,
	callOutcome callpayload.CallOutcomeProvider,
	ks *keyspace.KeySpace,
	in state.State,
) (product.Value, bool) {
	if len(generic.Sources) == 0 {
		return product.Value{}, false
	}
	source := generic.Sources[0]
	if source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return product.Value{}, false
	}
	site, ok := facts.CallSite(source.CallPoint)
	if !ok {
		return product.Value{}, false
	}
	name, ok := signatureID.nameForSite(site)
	if !ok {
		return genericForFunctionIteratorVariableValue(ctx, typeValues, generic, source, site, callOutcome, in)
	}
	iter, ok := genericForIterator(name, signatures)
	if !ok {
		return genericForFunctionIteratorVariableValue(ctx, typeValues, generic, source, site, callOutcome, in)
	}
	sourceIndex, ok := effect.ResolveParamIndex(iter.Source, site.ArgumentSourceCount())
	if !ok {
		return product.Value{}, false
	}
	argSource, ok := site.ArgumentSourceAt(sourceIndex)
	if !ok {
		return product.Value{}, false
	}
	assertedSourceType, hasAssertedSourceType := genericForAssertedIteratorSourceType(generic, sourceIndex, typeResolver)
	refinedSources := sourcevalue.WithExpressionRefinements(ctx.Registry, sources, expressionRefinements)
	sourceValue, ok := refinedSources.ValueOfSource(ctx.Point, argSource, in, ctx.Read)
	if !ok {
		return product.Value{}, false
	}
	if value, ok := genericForLiteralContainerVariableValue(ctx, iter, generic.VariableIndex, facts, refinedSources, sourceValue, argSource, ks, in); ok {
		return value, true
	}
	return luasourcevalue.IteratorVariableValue(ctx.Registry, typeValues, iter, generic.VariableIndex, sourceValue, assertedSourceType, hasAssertedSourceType)
}

// genericForFunctionIteratorVariableValue types a generic-for loop variable when
// the iterator source is a call returning a stateless iterator function. The Lua
// protocol calls that function each iteration and binds the loop variables to its
// results; the loop continues while the first result is non-nil. The variable at
// generic.VariableIndex therefore takes the iterator function's matching result
// type, with the first result narrowed to its non-nil form for the in-body value.
func genericForFunctionIteratorVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	generic cfgfacts.GenericForFact,
	source sourceprovenance.ASTSource,
	site factflow.CallSite,
	callOutcome callpayload.CallOutcomeProvider,
	in state.State,
) (product.Value, bool) {
	if !source.HasCallPoint || source.CallPoint == 0 || callOutcome == nil || ctx.Read == nil {
		return product.Value{}, false
	}
	callCtx := transfer.NodeContext{
		Graph:    ctx.Graph,
		Registry: ctx.Registry,
		Point:    source.CallPoint,
		Read:     ctx.Read,
	}
	if ctx.Graph != nil {
		callCtx.Node = ctx.Graph.Node(source.CallPoint)
	}
	outcome := callOutcome(callCtx, site.View(), ctx.Read(source.CallPoint), ctx.Read)
	var callResult product.Value
	found := false
	for _, result := range outcome.Results {
		if result.Index == 0 {
			callResult = result.Value
			found = true
			break
		}
	}
	if !found {
		return product.Value{}, false
	}
	iterType, ok := typevalue.TypeOf(ctx.Registry, callResult)
	if !ok {
		return product.Value{}, false
	}
	iterFunc, ok := iterType.(*typ.Function)
	if !ok || typ.IsAny(iterType) || typ.IsUnknown(iterType) {
		return product.Value{}, false
	}
	if generic.VariableIndex < 0 || generic.VariableIndex >= len(iterFunc.Returns) {
		return product.Value{}, false
	}
	resultType := iterFunc.Returns[generic.VariableIndex]
	if resultType == nil || typ.IsAny(resultType) || typ.IsUnknown(resultType) {
		return product.Value{}, false
	}
	if generic.VariableIndex == 0 {
		if optional, ok := resultType.(*typ.Optional); ok && optional.Inner != nil {
			resultType = optional.Inner
		}
	}
	return typeValues.FromTypeWithWitness(ctx.Registry, resultType), true
}

func genericForIterator(name string, signatures signaturelookup.Source) (iteration.Iterator, bool) {
	if sig, ok := signatures.Lookup(name); ok {
		return iteration.ActiveIterator(sig.Effect.Labels)
	}
	switch name {
	case "pairs":
		return iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}, true
	case "ipairs":
		return iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}, true
	default:
		return iteration.Iterator{}, false
	}
}

func genericForLiteralContainerVariableValue(
	ctx transfer.NodeContext,
	iter iteration.Iterator,
	variableIndex int,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	sourceValue product.Value,
	source factflow.ValueSource,
	ks *keyspace.KeySpace,
	in state.State,
) (product.Value, bool) {
	if variableIndex != 1 || iter.Kind != iteration.IterateIndexed || !source.HasExpr {
		return product.Value{}, false
	}
	if value, ok := genericForHeapContainerVariableValue(ctx, iter, ks, sourceValue, in); ok {
		return value, true
	}
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		return product.Value{}, false
	}
	var out product.Value
	seen := false
	for _, entry := range literal.Entries() {
		if !genericForDirectContainerElement(iter, entry) {
			continue
		}
		value, ok := sources.ValueOfSource(ctx.Point, entry.Source(), in, ctx.Read)
		if !ok {
			continue
		}
		if !seen {
			out = value
			seen = true
			continue
		}
		out = product.Join(ctx.Registry, out, value)
	}
	return out, seen
}

func genericForHeapContainerVariableValue(ctx transfer.NodeContext, iter iteration.Iterator, ks *keyspace.KeySpace, sourceValue product.Value, in state.State) (product.Value, bool) {
	id, ok := product.Get(ctx.Registry, sourceValue, identity.Key).ID()
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(ctx.Registry, id)
	if !product.Equal(ctx.Registry, object.Root(), sourceValue) {
		return product.Value{}, false
	}
	var out product.Value
	seen := false
	for key, value := range object.StaticMembers() {
		segs, ok := ks.SuffixSegments(key)
		if !ok || len(segs) != 1 || !genericForDirectContainerSegment(iter, segs[0]) {
			continue
		}
		if !seen {
			out = value
			seen = true
			continue
		}
		out = product.Join(ctx.Registry, out, value)
	}
	return out, seen
}

func genericForDirectContainerElement(iter iteration.Iterator, entry factflow.ObjectEntry) bool {
	segs := entry.Suffix().Segments
	if len(segs) != 1 {
		return false
	}
	return genericForDirectContainerSegment(iter, segs[0])
}

func genericForDirectContainerSegment(iter iteration.Iterator, seg segment.Segment) bool {
	switch iter.Kind {
	case iteration.IterateIndexed:
		return seg.Kind == segment.SegmentIndexInt
	default:
		return false
	}
}

func genericForAssertedIteratorSourceType(generic cfgfacts.GenericForFact, sourceIndex int, resolver *typeresolve.Resolver) (typ.Type, bool) {
	if sourceIndex < 0 || resolver == nil {
		return nil, false
	}
	arg := genericForCallArgument(generic, sourceIndex)
	if arg == nil {
		return nil, false
	}
	return assertedIteratorSourceType(arg, resolver)
}

func genericForCallArgument(generic cfgfacts.GenericForFact, sourceIndex int) ast.Expr {
	for _, expr := range generic.Exprs {
		call, ok := expr.(*ast.FuncCallExpr)
		if !ok || call == nil || sourceIndex >= len(call.Args) {
			continue
		}
		return call.Args[sourceIndex]
	}
	return nil
}

func assertedIteratorSourceType(expr ast.Expr, resolver *typeresolve.Resolver) (typ.Type, bool) {
	switch expr := expr.(type) {
	case *ast.CastExpr:
		t, ok := resolver.Type(expr.Type)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
			return nil, false
		}
		return t, true
	case *ast.NonNilAssertExpr:
		return assertedIteratorSourceType(expr.Expr, resolver)
	default:
		return nil, false
	}
}
