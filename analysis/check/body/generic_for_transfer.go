package body

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/projection"
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
		value, ok := genericForVariableValue(ctx, fact, facts, expressionRefinements, sources, signatures, signatureID, typeResolver, in)
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
	generic cfgfacts.GenericForFact,
	facts factflow.Facts,
	expressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement,
	sources sourcevalue.SourceValues,
	signatures signaturelookup.Source,
	signatureID *signatureIdentityResolver,
	typeResolver *typeresolve.Resolver,
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
		return product.Value{}, false
	}
	sig, ok := signatures.Lookup(name)
	if !ok {
		return product.Value{}, false
	}
	iter, ok := activeIterator(sig.Effect.Labels)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	sourceIndex, ok := effect.ResolveParamIndex(iter.Source, len(args))
	if !ok || sourceIndex < 0 || sourceIndex >= len(args) {
		return product.Value{}, false
	}
	assertedSourceType, hasAssertedSourceType := genericForAssertedIteratorSourceType(generic, sourceIndex, typeResolver)
	refinedSources := sourcevalue.WithExpressionRefinements(ctx.Registry, sources, expressionRefinements)
	sourceValue, ok := refinedSources.ValueOfSource(ctx.Point, args[sourceIndex], in, ctx.Read)
	if !ok {
		return product.Value{}, false
	}
	switch generic.VariableIndex {
	case 0:
		return iteratorKeyValue(ctx, iter, sourceValue, assertedSourceType, hasAssertedSourceType)
	case 1:
		return iteratorElementValue(ctx, sourceValue, assertedSourceType, hasAssertedSourceType)
	default:
		return product.Value{}, false
	}
}

func activeIterator(labels []effect.Label) (iteration.Iterator, bool) {
	for _, label := range labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case iteration.Iterator:
			return normalized, true
		case *iteration.Iterator:
			if normalized != nil {
				return *normalized, true
			}
		}
	}
	return iteration.Iterator{}, false
}

func iteratorKeyValue(ctx transfer.NodeContext, iter iteration.Iterator, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	switch iter.Kind {
	case iteration.IterateIndexed:
		return typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, typ.Integer), typ.Integer), true
	case iteration.IterateKeyed:
		if sourceType, ok := iteratorSourceType(ctx.Registry, sourceValue, assertedSourceType, hasAssertedSourceType); ok {
			if keyType, ok := projection.KeyOf(sourceType); ok {
				return typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, keyType), keyType), true
			}
		}
		return typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, typ.Any), typ.Any), true
	default:
		return product.Value{}, false
	}
}

func iteratorElementValue(ctx transfer.NodeContext, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (product.Value, bool) {
	sourceType, ok := iteratorSourceType(ctx.Registry, sourceValue, assertedSourceType, hasAssertedSourceType)
	if !ok {
		return product.Value{}, false
	}
	elem, ok := projection.ElementOf(sourceType)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, elem), elem), true
}

func iteratorSourceType(reg *axis.Registry, sourceValue product.Value, assertedSourceType typ.Type, hasAssertedSourceType bool) (typ.Type, bool) {
	if hasAssertedSourceType {
		return assertedSourceType, true
	}
	return objectLiteralEntryType(reg, sourceValue)
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
