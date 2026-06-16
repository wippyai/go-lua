package body

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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
		value, ok := genericForVariableValue(ctx, typeValues, fact, facts, expressionRefinements, sources, signatures, signatureID, typeResolver, in)
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
	iter, ok := iteration.ActiveIterator(sig.Effect.Labels)
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
	return luasourcevalue.IteratorVariableValue(ctx.Registry, typeValues, iter, generic.VariableIndex, sourceValue, assertedSourceType, hasAssertedSourceType)
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
