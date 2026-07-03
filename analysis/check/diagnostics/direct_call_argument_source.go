package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

type directCallArgumentSourceTypeResolution struct {
	Type             typ.Type
	ReadBoundary     boundaryValueReader
	Source           directCallArgumentSourceKind
	OK               bool
	UntrustedTopLike bool
}

type directCallArgumentSourceKind uint8

const (
	directCallArgumentSourceDeclared directCallArgumentSourceKind = iota
	directCallArgumentSourceFlow
	directCallArgumentSourceContract
	directCallArgumentSourceExplicitCast
	directCallArgumentSourceUntrusted
)

func (r directCallArgumentSourceTypeResolution) TypeMismatch(result *body.Result, point cfg.Point, want typ.Type) bool {
	if r.UntrustedTopLike {
		return boundaryProofTypeMismatch(result, point, r.Type, want, r.ReadBoundary)
	}
	return directCallArgumentTypeMismatch(result, point, r.Type, want, r.ReadBoundary)
}

func contextualParameterFlowObligationOwnedByCallSite(result *body.Result, context producerContext, resolution directCallArgumentSourceTypeResolution, arg ast.Expr) bool {
	if !context.callContextResult {
		return false
	}
	switch resolution.Source {
	case directCallArgumentSourceFlow, directCallArgumentSourceDeclared:
	default:
		return false
	}
	return contextualParameterArgumentOwnedByCallSite(result, context, arg)
}

func resolveDirectCallArgumentSourceType(
	result *body.Result,
	resolver typeannotation.Resolver,
	point cfg.Point,
	env guardEnv,
	fact semantics.CallFact,
	index int,
	arg ast.Expr,
	context producerContext,
	defs map[symbol.ID]*ast.FunctionExpr,
) directCallArgumentSourceTypeResolution {
	readBoundary := boundaryCallArgumentReader(fact, index, arg)
	if got, ok := concreteCastObligationType(result, resolver, point, env, arg); ok {
		return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceExplicitCast, OK: true}
	}
	if fn, ok := unwrapFunctionLiteralArgument(arg); ok {
		if got, ok := declaredArgumentExprType(result, resolver, arg); ok && !topLikeFunctionPlaceholder(got) {
			return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceDeclared, OK: true}
		}
		if !functionLiteralHasExplicitParamTypes(fn) {
			if got, ok := directCallArgumentFunctionSignatureType(result, point, index); ok {
				return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceContract, OK: true}
			}
		}
		if got, sourceRead, ok := directCallBoundaryArgumentValueSourceType(result, point, index); ok {
			return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: sourceRead, Source: directCallArgumentSourceContract, OK: true}
		}
	}
	if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, arg); ok {
		return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceUntrusted, OK: true, UntrustedTopLike: true}
	}
	if got, sourceRead, ok := directCallBoundaryArgumentValueSourceType(result, point, index); ok {
		if value, valueOK := sourceRead(result, point); valueOK {
			query := newDiagnosticQuery(result)
			if query.ValueHasUntrustedTopOrigin(value) && !query.ValueProofAdmissible(value, got) {
				return directCallArgumentSourceTypeResolution{Type: typ.Any, ReadBoundary: readBoundary, Source: directCallArgumentSourceUntrusted, OK: true, UntrustedTopLike: true}
			}
		}
		return directCallArgumentSourceTypeResolution{
			Type:             got,
			ReadBoundary:     sourceRead,
			Source:           directCallArgumentSourceFlow,
			OK:               true,
			UntrustedTopLike: boundaryValueHasUntrustedTopOrigin(result, point, sourceRead),
		}
	}
	if got, ok := directCallArgumentFlowExpressionTypeAllowNil(result, resolver, point, env, arg); ok {
		untrusted := boundaryValueHasUntrustedTopOrigin(result, point, readBoundary)
		if env.provesRuntimeType(result, point, arg, got) {
			return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceFlow, OK: true, UntrustedTopLike: untrusted}
		}
		if value, valueOK := readBoundary(result, point); valueOK && newDiagnosticQuery(result).ValueProofAdmissible(value, got) {
			return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceFlow, OK: true}
		}
	}
	if got, ok := directCallArgumentFlowExpressionTypeAllowNil(result, resolver, point, env, arg); ok {
		return directCallArgumentSourceTypeResolution{
			Type:             got,
			ReadBoundary:     readBoundary,
			Source:           directCallArgumentSourceFlow,
			OK:               true,
			UntrustedTopLike: boundaryValueHasUntrustedTopOrigin(result, point, readBoundary),
		}
	}
	if got, ok := directCallArgumentFunctionSignatureType(result, point, index); ok {
		return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceContract, OK: true}
	}
	if boundary, ok := boundaryCallArgumentReaderType(result, resolver, point, readBoundary, arg); ok {
		untrusted := boundaryValueHasUntrustedTopOrigin(result, point, readBoundary)
		if projectionHasNil(boundary) {
			if got, ok := directCallArgumentFlowExpressionType(result, resolver, point, env, arg); ok {
				return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceFlow, OK: true, UntrustedTopLike: untrusted}
			}
		}
		return directCallArgumentSourceTypeResolution{Type: boundary, ReadBoundary: readBoundary, Source: directCallArgumentSourceFlow, OK: true, UntrustedTopLike: untrusted}
	}
	if got, ok := directCallArgumentFlowExpressionType(result, resolver, point, env, arg); ok {
		return directCallArgumentSourceTypeResolution{
			Type:             got,
			ReadBoundary:     readBoundary,
			Source:           directCallArgumentSourceFlow,
			OK:               true,
			UntrustedTopLike: boundaryValueHasUntrustedTopOrigin(result, point, readBoundary),
		}
	}
	if got, ok := declaredArgumentExprType(result, resolver, arg); ok && !topLikeType(got) {
		return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceDeclared, OK: true}
	}
	if got, ok := directCallArgumentContractSourceType(result, context, fact, index, defs); ok {
		if topLikeType(got) {
			return directCallArgumentSourceTypeResolution{
				Type:             got,
				ReadBoundary:     untrustedAnyBoundaryReader(readBoundary),
				Source:           directCallArgumentSourceContract,
				OK:               true,
				UntrustedTopLike: true,
			}
		}
		return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceContract, OK: true}
	}
	if got, ok := boundaryCallArgumentSourceType(result, point, fact, index); ok {
		if topLikeType(got) {
			return directCallArgumentSourceTypeResolution{
				Type:             got,
				ReadBoundary:     untrustedAnyBoundaryReader(readBoundary),
				Source:           directCallArgumentSourceFlow,
				OK:               true,
				UntrustedTopLike: true,
			}
		}
		return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceFlow, OK: true}
	}
	if got, ok := staticExpressionType(result, resolver, arg); ok {
		return directCallArgumentSourceTypeResolution{Type: got, ReadBoundary: readBoundary, Source: directCallArgumentSourceFlow, OK: true}
	}
	return directCallArgumentSourceTypeResolution{ReadBoundary: readBoundary}
}

func directCallArgumentFunctionSignatureType(result *body.Result, point cfg.Point, index int) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	site, ok := result.CallSite(point)
	if !ok {
		return nil, false
	}
	source, ok := site.ArgumentSourceAt(index)
	if !ok || !source.HasExpr {
		return nil, false
	}
	if _, ok := result.ExpressionFunction(source.ExprRef); !ok {
		return nil, false
	}
	got, ok := result.SignatureArgumentTypeAtBoundary(point, source)
	if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func directCallBoundaryArgumentValueSourceType(result *body.Result, point cfg.Point, index int) (typ.Type, boundaryValueReader, bool) {
	if result == nil || index < 0 {
		return nil, nil, false
	}
	site, ok := result.CallSite(point)
	if !ok {
		return nil, nil, false
	}
	source, ok := site.ArgumentSourceAt(index)
	if !ok {
		return nil, nil, false
	}
	query := newDiagnosticQuery(result)
	read := directCallArgumentSourceValueReader(source)
	value, ok := read(result, point)
	if !ok {
		return nil, nil, false
	}
	got, ok := query.ValueTypeWithPresence(value)
	if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
		return nil, nil, false
	}
	return got, read, true
}

func directCallArgumentSourceValueReader(source factflow.ValueSource) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		query := newDiagnosticQuery(result)
		if source.Kind == factflow.ValueSourceCall {
			return query.SourceValueAtBoundary(point, source)
		}
		return query.SourceValueBeforeBoundary(point, source)
	}
}

func boundaryCallArgumentExpressionType(result *body.Result, point cfg.Point, arg ast.Expr) (typ.Type, bool) {
	if result == nil || arg == nil {
		return nil, false
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ExpressionValueAtBoundary(point, arg)
	if !ok {
		return nil, false
	}
	got, ok := query.ValueTypeWithPresence(value)
	if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func boundaryCallArgumentReaderType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, read boundaryValueReader, arg ast.Expr) (typ.Type, bool) {
	if result == nil || read == nil {
		return nil, false
	}
	value, ok := read(result, point)
	if !ok {
		return nil, false
	}
	query := newDiagnosticQuery(result)
	got, gotOK := query.ValueTypeWithPresence(value)
	gotUsable := gotOK && boundaryCallArgumentTypeUsable(got)
	if declared, ok := declaredArgumentExprType(result, resolver, arg); ok && !topLikeType(declared) {
		if refined, ok := query.RefineDeclaredType(declared, value); ok && boundaryCallArgumentTypeUsable(refined) {
			if gotUsable && boundaryCallValueProvesPresent(got, refined) {
				if completed, ok := overlayPresentDeclaredAggregateType(declared, got); ok && boundaryCallArgumentTypeUsable(completed) && !projectionHasNil(completed) {
					return completed, true
				}
				return got, true
			}
			if completed, ok := overlayDeclaredAggregateType(declared, refined); ok && boundaryCallArgumentTypeUsable(completed) {
				return completed, true
			}
			return refined, true
		}
	}
	if !gotUsable {
		return nil, false
	}
	return got, true
}

func boundaryCallArgumentTypeUsable(got typ.Type) bool {
	return got != nil && !topLikeType(got) && !refinement.ContainsFreeTypeParam(got)
}

func boundaryCallValueProvesPresent(got, refined typ.Type) bool {
	return got != nil && refined != nil && !projectionHasNil(got) && projectionHasNil(refined)
}

func directCallArgumentFlowExpressionType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, arg ast.Expr) (typ.Type, bool) {
	if boundary, ok := boundaryCallArgumentExpressionType(result, point, arg); ok && !projectionHasNil(boundary) {
		if completed, ok := declaredAggregateOverlayType(result, resolver, arg, boundary); ok {
			return completed, true
		}
		return boundary, true
	}
	refined, refinedOK := projectedFlowSourceType(result, resolver, point, env, arg)
	if refinedOK && !containsTypeParamSyntax(refined) && !projectionHasNil(refined) {
		if completed, ok := declaredAggregateOverlayType(result, resolver, arg, refined); ok {
			return completed, true
		}
		return refined, true
	}
	structural, structuralOK := projectedStructuralFlowSourceType(result, resolver, point, env, arg)
	if structuralOK && !containsTypeParamSyntax(structural) && !projectionHasNil(structural) {
		if completed, ok := declaredAggregateOverlayType(result, resolver, arg, structural); ok {
			return completed, true
		}
		return structural, true
	}
	if structuralOK && !projectionHasNil(structural) {
		return structural, true
	}
	if refinedOK && !projectionHasNil(refined) {
		return refined, true
	}
	if got, ok := guardedFlowExpressionType(result, resolver, point, env, arg); ok {
		return got, true
	}
	return nil, false
}

func guardedFlowExpressionType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, arg ast.Expr) (typ.Type, bool) {
	got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(arg)
	if !ok || got == nil || topLikeType(got) || containsTypeParamSyntax(got) || projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func directCallArgumentFlowExpressionTypeAllowNil(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, arg ast.Expr) (typ.Type, bool) {
	if boundary, ok := boundaryCallArgumentExpressionType(result, point, arg); ok {
		return boundary, true
	}
	got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(arg)
	if !ok || got == nil || topLikeType(got) || containsTypeParamSyntax(got) {
		return nil, false
	}
	return got, true
}

func declaredAggregateOverlayType(result *body.Result, resolver typeannotation.Resolver, arg ast.Expr, overlay typ.Type) (typ.Type, bool) {
	declared, ok := declaredPathType(result, resolver, arg)
	if !ok || topLikeType(declared) || projectionHasNil(declared) {
		return nil, false
	}
	return overlayDeclaredAggregateType(declared, overlay)
}

func overlayDeclaredAggregateType(declared, overlay typ.Type) (typ.Type, bool) {
	if completed, ok := typetable.OverlayRecordMembers(declared, overlay); ok {
		return completed, true
	}
	if rec, ok := unwrap.Alias(declared).(*typ.Recursive); ok && rec.Body != nil && rec.Body != rec {
		return typetable.OverlayRecordMembers(rec.Body, overlay)
	}
	return nil, false
}

func overlayPresentDeclaredAggregateType(declared, overlay typ.Type) (typ.Type, bool) {
	if projectionHasNil(declared) {
		present := projectionWithoutNil(declared)
		if present == nil || typ.IsNever(present) {
			return nil, false
		}
		declared = present
	}
	return overlayDeclaredAggregateType(declared, overlay)
}

func directCallArgumentContractSourceType(
	result *body.Result,
	context producerContext,
	fact semantics.CallFact,
	index int,
	defs map[symbol.ID]*ast.FunctionExpr,
) (typ.Type, bool) {
	if result == nil || index < 0 || index >= len(fact.ArgumentSources) {
		return nil, false
	}
	source := fact.ArgumentSources[index]
	if source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return nil, false
	}
	sourceFact, ok := result.Call(source.CallPoint)
	if !ok {
		return nil, false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok {
		return nil, false
	}
	var def *ast.FunctionExpr
	if defs != nil && site.CalleeSymbol() != 0 {
		def = defs[site.CalleeSymbol()]
	}
	contract, _, ok := directCallResultContract(result, context, source.CallPoint, sourceFact, site, def, defs)
	if !ok {
		return nil, false
	}
	if got, ok := contract.returnType(source.ResultIndex); ok && !refinement.ContainsFreeTypeParam(got) {
		return got, true
	}
	got, ok := contract.declaredReturnType(source.ResultIndex)
	if !ok || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func directCallArgumentTypeMismatch(result *body.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader) bool {
	if !boundaryTypeMismatch(result, point, got, want, nil) {
		return false
	}
	if read != nil {
		if value, ok := read(result, point); ok && newDiagnosticQuery(result).ValueProofAdmissible(value, want) {
			return false
		}
	}
	return true
}

func boundaryCallArgumentSourceType(result *body.Result, point cfg.Point, fact semantics.CallFact, index int) (typ.Type, bool) {
	if index < 0 || index >= len(fact.ArgumentSources) {
		return nil, false
	}
	return newDiagnosticQuery(result).SourceType(point, fact.ArgumentSources[index])
}

func boundaryCallArgumentReader(fact semantics.CallFact, index int, argumentExpr ast.Expr) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		query := newDiagnosticQuery(result)
		if result != nil && argumentExpr != nil {
			if p, ok := result.ExpressionPath(argumentExpr); ok && !p.IsEmpty() {
				if value, ok := query.ExpressionValueBeforeBoundary(point, argumentExpr); ok {
					return value, true
				}
			}
		}
		if result != nil && index >= 0 {
			if site, ok := result.CallSite(point); ok {
				if source, ok := site.ArgumentSourceAt(index); ok {
					if source.Kind == factflow.ValueSourceCall {
						if value, ok := query.SourceValueAtBoundary(point, source); ok {
							return value, true
						}
					}
					if value, ok := query.SourceValueBeforeBoundary(point, source); ok {
						return value, true
					}
				}
			}
		}
		if index >= 0 && index < len(fact.ArgumentSources) {
			if value, ok := boundaryValueFromASTSource(fact.ArgumentSources[index])(result, point); ok {
				return value, true
			}
		}
		return boundaryValueFromExpr(argumentExpr)(result, point)
	}
}
