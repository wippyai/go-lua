package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// annotationAssignability reports clear contradictions between a local
// annotation and a syntactically known source literal or scalar operator
// expression. Broader flow-to-type projection belongs in later producers once
// the relevant value axes own it.
type annotationAssignability producerContext

func (p annotationAssignability) Produce(result *body.Result, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := producerContext(p).guardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !result.PointReachable(point) || !guardEnvReachableAt(envs, point) {
			continue
		}
		if fact, ok := result.LocalAssignment(point); ok {
			if d, ok := p.localAssignment(result, point, fact, envs[point], defs); ok {
				out = append(out, d)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if d, ok := p.pathAssignment(result, point, fact, envs[point]); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

// underSuppliedTargetAssignment reports a destructuring target of an INITIALIZED
// multi-assignment whose value list supplies fewer values than there are targets,
// so the target is nil-filled. Such a target has no source expression of its own;
// its bound value is read directly. A nil-filled target against a non-optional
// annotation is an error at the declaration, independent of how many targets or
// values the assignment has. A declaration with no initializer (len(Exprs) == 0)
// is left to flow-sensitive use analysis, not reported here.
func (p annotationAssignability) underSuppliedTargetAssignment(result *body.Result, point cfg.Point, fact semantics.LocalAssignmentFact) (diagnostic.Diagnostic, bool) {
	if len(fact.Exprs) == 0 || !fact.HasSymbol {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := lowerType(fact.Type, p.resolver)
	if !ok || want == nil || typ.IsAny(want) || typ.IsUnknown(want) || projectionHasNil(want) {
		return diagnostic.Diagnostic{}, false
	}
	value, ok := result.SymbolValueAtBoundary(point, fact.Symbol)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got, ok := newDiagnosticQuery(result).ValueTypeWithPresence(value)
	if !ok || got == nil {
		return diagnostic.Diagnostic{}, false
	}
	// Fire only for a pure nil-fill: a target supplied no value at all. A target
	// that received a real value of an optional or mismatched type (an in-range
	// result slot) is not under-supplied; its mismatch is reported by the
	// call-result/value paths, not here.
	if !typ.Nil.Equals(got) {
		return diagnostic.Diagnostic{}, false
	}
	return underSuppliedTargetDiagnostic(fact.Name, want, got, fact.Type, fact.Source), true
}

func (p annotationAssignability) localAssignment(result *body.Result, point cfg.Point, fact semantics.LocalAssignmentFact, env guardEnv, directDefs map[symbol.ID]*ast.FunctionExpr) (diagnostic.Diagnostic, bool) {
	if fact.Type == nil {
		return diagnostic.Diagnostic{}, false
	}
	if fact.Expr == nil {
		return p.underSuppliedTargetAssignment(result, point, fact)
	}
	want, ok := lowerType(fact.Type, p.resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if got, ok := explicitTopLikeCastType(p.resolver, fact.Expr); ok {
		extra := explicitTopLikeCastEvidence(ast.SpanOf(fact.Expr), want, fact.Expr)
		return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type, extra...), true
	}
	if diag, ok := p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env); ok {
		return diag, true
	}
	if got, ok := currentFunctionDefinitionValueType(result, producerContext(p), point, fact.Expr); ok {
		if boundaryTypeMismatch(result, point, got, want, nil) {
			return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
		}
		return diagnostic.Diagnostic{}, false
	}
	if got, ok := reassignedCallResultFieldBoundaryType(result, p.resolver, p.flow, point, fact.Expr, directDefs); ok {
		if boundaryTypeMismatch(result, point, got, want, nil) {
			extra := reassignedCallResultFieldEvidence(result, p.resolver, p.flow, point, fact.Expr)
			return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type, extra...), true
		}
		return diagnostic.Diagnostic{}, false
	}
	if got, ok := callResultWitnessProvenMismatchType(result, point, fact.Source, want); ok &&
		(!unstableCallResultRootFieldPath(result, producerContext(p), point, fact.Expr) || callResultRootProvenPresent(result, env, fact.Expr)) {
		return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
	}
	if directCallResultOwner(result, fact.Source) {
		if got, ok := directFunctionCurrentReturnPathType(result, producerContext(p), fact.Source, directDefs); ok {
			if boundaryTypeMismatch(result, fact.Source.CallPoint, got, want, nil) {
				return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
			}
		}
	}
	if directCallResultOwner(result, fact.Source) && !directCallSourceHasTypedSignature(result, fact.Source) {
		if !callResultWitnessProvenMismatch(result, point, fact.Source, want) {
			return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
		}
	}
	if _, topLikeCast := explicitTopLikeCastType(p.resolver, fact.Expr); !topLikeCast && env.provesRuntimeType(result, point, fact.Expr, want) {
		return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
	}
	sourceResolution := resolveLocalAssignmentSourceType(result, producerContext(p), p.resolver, p.flow, point, fact, env, directDefs)
	if !sourceResolution.OK {
		return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
	}
	got := sourceResolution.Type
	if sourceResolution.AllowsFlowRefinement(got) {
		got = refineAssignmentSourceType(result, point, sourceResolution.RefineExpr(fact.Expr), got)
	}
	readBoundary := boundaryValueFromASTSource(fact.Source)
	if sourceResolution.CastInnerExpr != nil {
		readBoundary = boundaryValueFromExpr(sourceResolution.CastInnerExpr)
	}
	mismatchBoundary := sourceResolution.MismatchBoundary(readBoundary)
	mismatch := boundaryTypeMismatch(result, point, got, want, mismatchBoundary)
	if sourceResolution.UntrustedTopLike {
		mismatch = boundaryProofTypeMismatch(result, point, got, want, mismatchBoundary)
	}
	if !mismatch && boundaryValueNeedsValidationProof(result, point, mismatchBoundary, want) {
		mismatch = true
		sourceResolution.UntrustedTopLike = true
		if !clearMismatch(result, got, want) {
			got = typ.Any
		}
	}
	if mismatch {
		if sourceResolution.OptionalMemberProjection && optionalUnknownType(got) && !optionalUnknownMemberHasInvalidation(result, p.flow, point, fact.Expr) {
			return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
		}
		if !sourceResolution.OptionalMemberProjection && callResultFieldOptionalImprecision(result, producerContext(p), point, fact.Expr, got, want) {
			return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
		}
		if !sourceResolution.OptionalIndexProjection && expressionHasMissingMemberRead(result, p.resolver, point, env, fact.Expr) {
			if !sourceResolution.OptionalMemberProjection || len(optionalReceiverCauseEvidence(result, p.resolver, point, env, fact.Expr)) == 0 {
				return diagnostic.Diagnostic{}, false
			}
		}
		if !sourceResolution.UntrustedTopLike {
			if diag, ok := p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env); ok {
				return diag, true
			}
		}
		var extra []diagnostic.Evidence
		if sourceResolution.ReassignedCallResultProjection {
			extra = append(extra, reassignedCallResultFieldEvidence(result, p.resolver, p.flow, point, fact.Expr)...)
		}
		extra = append(extra, optionalReceiverCauseEvidence(result, p.resolver, point, env, fact.Expr)...)
		extra = append(extra, callInvalidatedPathEvidence(result, point, fact.Expr)...)
		extra = append(extra, boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(fact.Expr), exprEvidenceName(fact.Expr), want, readBoundary)...)
		if sourceResolution.UntrustedTopLike {
			extra = append(extra, explicitTopLikeCastEvidence(ast.SpanOf(fact.Expr), want, fact.Expr)...)
		}
		if sourceResolution.OptionalIndexProjection && !hasMissingBoundaryProofEvidence(extra) {
			extra = append(extra, missingIndexReadProofEvidence(ast.SpanOf(fact.Expr), want))
		}
		return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type, extra...), true
	}
	return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
}

func optionalUnknownType(t typ.Type) bool {
	if t == nil || !projectionHasNil(t) {
		return false
	}
	inner := projectionWithoutNil(t)
	return inner == nil || typ.IsNever(inner) || typ.IsAny(inner) || typ.IsUnknown(inner)
}

func guardedPresentExpressionType(result *body.Result, env guardEnv, expr ast.Expr, got typ.Type) (typ.Type, bool) {
	if result == nil || expr == nil || got == nil || !projectionHasNil(got) {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.IsEmpty() || (!env.hasPresent(accessPath) && !env.hasTruthy(accessPath)) {
		return nil, false
	}
	withoutNil := projectionWithoutNil(got)
	if withoutNil == nil || typ.IsNever(withoutNil) {
		return nil, false
	}
	return withoutNil, true
}

func optionalUnknownMemberHasInvalidation(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr) bool {
	if result == nil || expr == nil {
		return false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 {
		return false
	}
	_, declarationPoint, ok := dominatingRootLocalAssignment(result, flow, point, accessPath.Symbol)
	return ok && pathInvalidatedBetween(result, flow, declarationPoint, point, accessPath)
}

type localAssignmentSourceTypeResolution struct {
	Type                           typ.Type
	CastInnerExpr                  ast.Expr
	OK                             bool
	OptionalIndexProjection        bool
	PresenceAwareSourceProjection  bool
	OptionalMemberProjection       bool
	UntrustedTopLike               bool
	DeclarationProjection          bool
	ReassignedCallResultProjection bool
}

func (r localAssignmentSourceTypeResolution) RefineExpr(fallback ast.Expr) ast.Expr {
	if r.CastInnerExpr != nil {
		return r.CastInnerExpr
	}
	return fallback
}

func (r localAssignmentSourceTypeResolution) AllowsFlowRefinement(t typ.Type) bool {
	return !r.OptionalIndexProjection &&
		!r.PresenceAwareSourceProjection &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t)
}

func (r localAssignmentSourceTypeResolution) MismatchBoundary(readBoundary boundaryValueReader) boundaryValueReader {
	if r.DeclarationProjection || r.OptionalIndexProjection || r.PresenceAwareSourceProjection {
		return nil
	}
	return readBoundary
}

func resolveLocalAssignmentSourceType(
	result *body.Result,
	context producerContext,
	resolver typeannotation.Resolver,
	flow *diagnosticFlowCache,
	point cfg.Point,
	fact semantics.LocalAssignmentFact,
	env guardEnv,
	directDefs map[symbol.ID]*ast.FunctionExpr,
) localAssignmentSourceTypeResolution {
	if got, ok := explicitTopLikeCastType(resolver, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, UntrustedTopLike: true}
	}
	if got, ok := valueexpr.LiteralType(fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := projectedOptionalIndexType(result, resolver, point, fact.Expr); ok {
		if narrowed, narrowedOK := guardedPresentExpressionType(result, env, fact.Expr, got); narrowedOK {
			return localAssignmentSourceTypeResolution{Type: narrowed, OK: true, PresenceAwareSourceProjection: true}
		}
		return localAssignmentSourceTypeResolution{Type: got, OK: true, OptionalIndexProjection: true}
	}
	if got, ok := concreteCastObligationType(result, resolver, point, env, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := exactNilBoundaryExpressionType(result, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true}
	}
	if got, ok := localScalarOperatorSourceType(result, resolver, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := currentFunctionDefinitionValueType(result, context, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true}
	}
	if got, ok := result.FunctionValueTypeAtBoundary(point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := reassignedCallResultFieldBoundaryType(result, resolver, flow, point, fact.Expr, directDefs); ok {
		return localAssignmentSourceTypeResolution{
			Type:                           got,
			OK:                             true,
			PresenceAwareSourceProjection:  true,
			ReassignedCallResultProjection: true,
		}
	}
	if got, ok := optionalCallResultRootFieldReadType(result, resolver, flow, point, env, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true, OptionalMemberProjection: true}
	}
	if got, ok := dominatingCallResultPathType(result, context, point, fact.Expr, directDefs); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := dominatingCallResultFieldSourceType(result, context, point, fact.Expr, fact.Source); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := optionalCallResultMemberReadType(result, resolver, flow, point, env, fact.Expr, directDefs); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true, OptionalMemberProjection: true}
	}
	if got, ok := optionalMemberReadType(result, resolver, flow, point, env, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true, OptionalMemberProjection: true}
	}
	callResultRoot := callResultRootFieldPath(result, flow, point, fact.Expr)
	callResultRootPresent := callResultRootProvenPresent(result, env, fact.Expr)
	unstableCallResult := unstableCallResultRootFieldPath(result, context, point, fact.Expr) && !callResultRootPresent
	if got, ok := projectedFlowSourceType(result, resolver, point, env, fact.Expr); ok && !unstableCallResult && (!callResultRoot || callResultRootPresent) {
		if optionalIndexReadRequiresProof(result, resolver, point, fact.Expr) && !topLikeType(got) {
			if !projectionHasNil(got) {
				if narrowed, narrowedOK := guardedPresentExpressionType(result, env, fact.Expr, normalize.Optional(got)); narrowedOK {
					return localAssignmentSourceTypeResolution{Type: narrowed, OK: true, PresenceAwareSourceProjection: true}
				}
				got = normalize.Optional(got)
			}
			return localAssignmentSourceTypeResolution{Type: got, OK: true, OptionalIndexProjection: true}
		}
		if projectionHasNil(got) {
			if declared, declaredOK := dominatingDeclarationProjectionType(result, resolver, flow, point, fact.Expr); declaredOK && !projectionHasNil(declared) {
				return localAssignmentSourceTypeResolution{Type: declared, OK: true, DeclarationProjection: true}
			}
		}
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := dominatingDeclarationProjectionType(result, resolver, flow, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, DeclarationProjection: true}
	}
	if got, ok := boundaryExpressionTypeWithPresence(result, point, fact.Expr); ok {
		resolution := localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true}
		if optionalIndexReadRequiresProof(result, resolver, point, fact.Expr) && !topLikeType(got) {
			resolution.OptionalIndexProjection = true
		}
		return resolution
	}
	if got, ok := boundaryExpressionConcreteType(result, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true}
	}
	if got, ok := freshRecordAbsentFieldSourceType(result, resolver, point, env, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if unstableCallResult || (callResultRoot && !callResultRootPresent) {
		return localAssignmentSourceTypeResolution{Type: normalize.Optional(typ.Unknown), OK: true, PresenceAwareSourceProjection: true, OptionalMemberProjection: true}
	}
	if got, ok := untrustedAnnotatedIdentifierType(result, resolver, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, UntrustedTopLike: true}
	}
	if got, ok := sourceExpressionTypeWithPresence(result, point, fact.Source); ok {
		resolution := localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true}
		if optionalIndexReadRequiresProof(result, resolver, point, fact.Expr) && !topLikeType(got) {
			resolution.OptionalIndexProjection = true
		}
		return resolution
	}
	query := newDiagnosticQuery(result)
	if value, ok := result.LocalAssignmentSourceValueAtBoundary(point, fact.Source); ok {
		if got, gotOK := query.ValueTypeWithPresence(value); gotOK {
			if optionalIndexReadRequiresProof(result, resolver, point, fact.Expr) && !topLikeType(got) {
				if !projectionHasNil(got) {
					got = normalize.Optional(got)
				}
				return localAssignmentSourceTypeResolution{Type: got, OK: true, OptionalIndexProjection: true}
			}
			return localAssignmentSourceTypeResolution{Type: got, OK: true}
		}
	}
	if got, ok := query.SourceType(point, fact.Source); ok {
		if optionalIndexReadRequiresProof(result, resolver, point, fact.Expr) && !topLikeType(got) {
			if !projectionHasNil(got) {
				got = normalize.Optional(got)
			}
			return localAssignmentSourceTypeResolution{Type: got, OK: true, OptionalIndexProjection: true}
		}
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := annotatedIdentifierType(result, resolver, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := explicitTopLikeExpressionType(result, resolver, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := explicitTopLikeCallFactSourceType(result, resolver, fact.Source); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := callInvalidatedBoundaryExprType(result, resolver, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	return localAssignmentSourceTypeResolution{}
}

func callResultRootFieldPath(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr) bool {
	if result == nil || expr == nil {
		return false
	}
	accessPath, ok := result.ExpressionPath(expr)
	return ok &&
		accessPath.Symbol != 0 &&
		len(accessPath.Segments) != 0 &&
		allFieldSegments(accessPath.Segments) &&
		rootInitializedByDominatingCall(result, flow, point, accessPath.Symbol)
}

func unstableCallResultRootFieldPath(result *body.Result, context producerContext, point cfg.Point, expr ast.Expr) bool {
	if !callResultRootFieldPath(result, context.flow, point, expr) {
		return false
	}
	accessPath, _ := result.ExpressionPath(expr)
	source, ok := dominatingCallSourceForRoot(result, context.flow, point, accessPath.Symbol)
	if !ok {
		return false
	}
	if directCallReturnIsWrapperCall(result, source, nil) && !wrapperProviderReplacementDominatesCall(result, context, source) {
		return true
	}
	return callCalleeParentHasNonDominatingAssignment(result, source)
}

func callResultRootProvenPresent(result *body.Result, env guardEnv, expr ast.Expr) bool {
	if result == nil || expr == nil {
		return false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return false
	}
	root := accessPath.RootOnly()
	return env.hasPresent(root) || env.hasTruthy(root)
}

func optionalCallResultMemberReadType(
	result *body.Result,
	resolver typeannotation.Resolver,
	flow *diagnosticFlowCache,
	point cfg.Point,
	env guardEnv,
	expr ast.Expr,
	directDefs map[symbol.ID]*ast.FunctionExpr,
) (typ.Type, bool) {
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 || callResultRootProvenPresent(result, env, expr) {
		return nil, false
	}
	fact, _, ok := dominatingRootLocalAssignment(result, flow, point, accessPath.Symbol)
	if !ok || fact.Source.Kind != sourceprovenance.SourceCall {
		return nil, false
	}
	root, ok := directCallReturnSourceType(result, resolver, fact.Source, directDefs)
	if !ok || !projectionHasNil(root) {
		return nil, false
	}
	present := projectionWithoutNil(root)
	if present == nil || typ.IsNever(present) {
		return normalize.Optional(typ.Unknown), true
	}
	got, ok := expectedTypeAtSegments(present, accessPath.Segments)
	if !ok || got == nil || typ.IsNever(got) {
		return normalize.Optional(typ.Unknown), true
	}
	return normalize.Optional(got), true
}

func optionalCallResultRootFieldReadType(
	result *body.Result,
	resolver typeannotation.Resolver,
	flow *diagnosticFlowCache,
	point cfg.Point,
	env guardEnv,
	expr ast.Expr,
) (typ.Type, bool) {
	if !callResultRootFieldPath(result, flow, point, expr) || callResultRootProvenPresent(result, env, expr) {
		return nil, false
	}
	got, ok := projectedFlowSourceType(result, resolver, point, env, expr)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) || !projectionHasNil(got) {
		return nil, false
	}
	inner := projectionWithoutNil(got)
	if inner == nil || typ.IsNever(inner) || typ.IsAny(inner) || typ.IsUnknown(inner) {
		return nil, false
	}
	return got, true
}

func exactNilBoundaryExpressionType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	got, ok := boundaryExpressionTypeWithPresence(result, point, expr)
	if ok && typ.Nil.Equals(got) {
		return got, true
	}
	if result == nil || expr == nil {
		return nil, false
	}
	if _, ok := expr.(*ast.IdentExpr); !ok {
		return nil, false
	}
	value, ok := newDiagnosticQuery(result).ExpressionValueBeforeBoundary(point, expr)
	if !ok {
		return nil, false
	}
	got, ok = newDiagnosticQuery(result).ValueTypeWithPresence(value)
	return got, ok && typ.Nil.Equals(got)
}

func (p annotationAssignability) pathAssignment(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact, env guardEnv) (diagnostic.Diagnostic, bool) {
	if fact.Target == nil || fact.Value == nil {
		return diagnostic.Diagnostic{}, false
	}
	if d, ok := p.optionalAssignmentTarget(result, point, fact.Target, env); ok {
		return d, true
	}
	want, ok := assignmentTargetType(result, p.resolver, point, fact)
	if !ok || topLikeType(want) || refinement.ContainsFreeTypeParam(want) {
		return diagnostic.Diagnostic{}, false
	}
	if mismatch, ok := objectLiteralMemberMismatch(result, p.resolver, point, fact.Value, want, env); ok {
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
		extra = append(extra, mismatch.missingMemberEvidence()...)
		extra = append(extra, mismatch.unionArmEvidence...)
		return pathAssignmentDiagnostic(fact.Target, mismatch.expr, mismatch.got, mismatch.want, extra...), true
	}
	sourceResolution := resolvePathAssignmentSourceType(result, producerContext(p), p.resolver, p.flow, point, fact, env)
	if !sourceResolution.OK {
		return diagnostic.Diagnostic{}, false
	}
	got := sourceResolution.Type
	readBoundary := boundaryValueFromASTSource(fact.Source)
	if sourceResolution.CastInnerExpr != nil {
		readBoundary = boundaryValueFromExpr(sourceResolution.CastInnerExpr)
	}
	if !sourceResolution.TypeMismatch(result, point, want, readBoundary) {
		return diagnostic.Diagnostic{}, false
	}
	if sourceResolution.UntrustedTopLike {
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(fact.Value), exprEvidenceName(fact.Value), want, readBoundary)
		return pathAssignmentDiagnostic(fact.Target, fact.Value, got, want, extra...), true
	}
	if expressionHasMissingMemberRead(result, p.resolver, point, env, fact.Value) {
		return diagnostic.Diagnostic{}, false
	}
	extra := callInvalidatedPathEvidence(result, point, fact.Value)
	extra = append(extra, boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(fact.Value), exprEvidenceName(fact.Value), want, readBoundary)...)
	extra = append(optionalReceiverCauseEvidence(result, p.resolver, point, env, fact.Value), extra...)
	return pathAssignmentDiagnostic(fact.Target, fact.Value, got, want, extra...), true
}

type pathAssignmentSourceTypeResolution struct {
	Type             typ.Type
	CastInnerExpr    ast.Expr
	OK               bool
	UntrustedTopLike bool
}

func (r pathAssignmentSourceTypeResolution) TypeMismatch(result *body.Result, point cfg.Point, want typ.Type, readBoundary boundaryValueReader) bool {
	if r.UntrustedTopLike {
		return boundaryProofTypeMismatch(result, point, r.Type, want, readBoundary)
	}
	return boundaryTypeMismatch(result, point, r.Type, want, readBoundary)
}

func resolvePathAssignmentSourceType(
	result *body.Result,
	context producerContext,
	resolver typeannotation.Resolver,
	flow *diagnosticFlowCache,
	point cfg.Point,
	fact semantics.OrdinaryAssignmentFact,
	env guardEnv,
) pathAssignmentSourceTypeResolution {
	if got, ok := concreteCastObligationType(result, resolver, point, env, fact.Value); ok {
		return pathAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := dominatingCallInitializerType(result, context, resolver, flow, point, fact.Value); ok {
		return pathAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	got, ok := assignmentValueType(result, resolver, point, fact.Value, fact.Source)
	if !ok {
		if untrustedGot, untrustedOK := untrustedTopLikeExpressionTypeAt(result, resolver, point, fact.Value); untrustedOK {
			return pathAssignmentSourceTypeResolution{Type: untrustedGot, OK: true, UntrustedTopLike: true}
		}
		return pathAssignmentSourceTypeResolution{}
	}
	_, untrustedTopLike := untrustedTopLikeExpressionTypeAt(result, resolver, point, fact.Value)
	return pathAssignmentSourceTypeResolution{Type: got, OK: true, UntrustedTopLike: untrustedTopLike}
}

func dominatingCallInitializerType(
	result *body.Result,
	context producerContext,
	resolver typeannotation.Resolver,
	flow *diagnosticFlowCache,
	point cfg.Point,
	expr ast.Expr,
) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return nil, false
	}
	fact, declarationPoint, ok := dominatingRootLocalAssignment(result, flow, point, accessPath.Symbol)
	if !ok || fact.Expr == nil {
		return nil, false
	}
	if fact.Source.Kind != sourceprovenance.SourceCall || !fact.Source.HasCallPoint {
		return nil, false
	}
	if pathInvalidatedBetween(result, flow, declarationPoint, point, accessPath.RootOnly()) {
		return nil, false
	}
	env := context.guardEnv(result, fact.Source.CallPoint)
	got, ok := newFlowExpressionTyper(result, resolver, fact.Source.CallPoint, env).typeOf(fact.Expr)
	if !ok || got == nil || typ.IsNever(got) || topLikeType(got) {
		return nil, false
	}
	return got, true
}

func (p annotationAssignability) optionalAssignmentTarget(result *body.Result, point cfg.Point, target ast.Expr, env guardEnv) (diagnostic.Diagnostic, bool) {
	attr, ok := assignmentTargetAttr(target)
	if !ok || attr.Object == nil {
		return diagnostic.Diagnostic{}, false
	}
	container, receiverType, ok := optionalAssignmentTargetContainer(result, p.resolver, point, attr.Object, env)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	if !projectionHasNil(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	return optionalAssignmentTargetDiagnostic(container, target, receiverType), true
}

func optionalAssignmentTargetContainer(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, object ast.Expr, env guardEnv) (ast.Expr, typ.Type, bool) {
	if result == nil || object == nil {
		return nil, nil, false
	}
	if attr, ok := object.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyIndex {
		return optionalAssignmentTargetContainer(result, resolver, point, attr.Object, env)
	}
	receiverType, ok := declaredPathType(result, resolver, object)
	if !ok || receiverType == nil {
		return nil, nil, false
	}
	if flow, flowOK := newFlowExpressionTyper(result, resolver, point, env).typeOf(object); flowOK &&
		flow != nil &&
		!typ.IsNever(flow) &&
		!topLikeType(flow) {
		receiverType = flow
	}
	if objectPath, ok := result.ExpressionPath(object); ok && env.hasPresent(objectPath) {
		if present := projectionWithoutNil(receiverType); present != nil && !typ.IsNever(present) {
			receiverType = present
		}
	}
	return object, receiverType, true
}

func assignmentTargetAttr(target ast.Expr) (*ast.AttrGetExpr, bool) {
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		return t, true
	case *ast.CastExpr:
		return assignmentTargetAttr(t.Expr)
	case *ast.NonNilAssertExpr:
		return nil, false
	default:
		return nil, false
	}
}

func expressionHasMissingMemberRead(
	result *body.Result,
	resolver typeannotation.Resolver,
	point cfg.Point,
	env guardEnv,
	expr ast.Expr,
) bool {
	if result == nil || expr == nil {
		return false
	}
	typers := memberReadTypers{
		narrowed: newStructuralFlowExpressionTyper(result, resolver, point, env),
		base:     newStructuralFlowExpressionTyper(result, resolver, point, guardEnv{}),
		result:   result,
		point:    point,
	}
	collector := newMemberReadCollector()
	memberRead(producerContext{resolver: resolver}).walk(expr, typers, collector)
	return len(collector.Diagnostics()) != 0
}

func optionalReceiverCauseEvidence(
	result *body.Result,
	resolver typeannotation.Resolver,
	point cfg.Point,
	env guardEnv,
	expr ast.Expr,
) []diagnostic.Evidence {
	if result == nil || expr == nil {
		return nil
	}
	typer := newFlowExpressionTyper(result, resolver, point, env)
	var out []diagnostic.Evidence
	seen := make(map[string]struct{})
	var walk func(ast.Expr)
	walk = func(current ast.Expr) {
		switch e := current.(type) {
		case *ast.AttrGetExpr:
			walk(e.Object)
			receiverName := exprEvidenceNameOK(e.Object)
			memberName := attrKeyEvidenceName(e)
			if receiverName != "" {
				if receiverType, ok := typer.typeOf(e.Object); ok && receiverType != nil &&
					!typ.Nil.Equals(receiverType) && projectionHasNil(receiverType) {
					span := spanWithEvidenceName(ast.SpanOf(e.Object), receiverName)
					message := optionalReceiverReadEvidence(receiverName, memberName)
					key := message + "@" + spanKey(span)
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						out = append(out, diagnostic.Evidence{
							Kind:    diagnostic.EvidenceAbstractFact,
							Trust:   diagnostic.TrustProven,
							Span:    span,
							Message: message,
						})
					}
				}
			}
			walk(e.Key)
		case *ast.FuncCallExpr:
			if e.Receiver != nil {
				walk(e.Receiver)
			}
			if e.Func != nil {
				walk(e.Func)
			}
			for _, arg := range e.Args {
				walk(arg)
			}
		case *ast.CastExpr:
			walk(e.Expr)
		case *ast.NonNilAssertExpr:
			walk(e.Expr)
		}
	}
	walk(expr)
	return out
}

func spanKey(span diagnostic.Span) string {
	return fmt.Sprintf("%d:%d:%d:%d", span.StartLine, span.StartCol, span.EndLine, span.EndCol)
}

// callResultWitnessProvenMismatch reports whether the converged result value of
// a call source carries a concrete type witness that provably contradicts want.
// A call to a body-defined function without an explicit return annotation has no
// declared contract, so this is the proof path for assigning its inferred result
// to an annotated local: the result type is taken from the summary's converged
// return value, and only a non-gradual witness contradiction reports.
func callResultWitnessProvenMismatch(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource, want typ.Type) bool {
	_, ok := callResultWitnessProvenMismatchType(result, point, source, want)
	return ok
}

func callResultWitnessProvenMismatchType(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource, want typ.Type) (typ.Type, bool) {
	if result == nil || want == nil {
		return nil, false
	}
	return newDiagnosticQuery(result).SourceWitnessProvenMismatchType(point, source, want)
}

func directCallSourceHasTypedSignature(result *body.Result, source sourceprovenance.ASTSource) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok {
		return false
	}
	_, ok = result.CallSignatureType(site)
	return ok
}

func (p annotationAssignability) objectLiteralAssignment(result *body.Result, point cfg.Point, name string, want typ.Type, expr ast.Expr, annotation ast.TypeExpr, env guardEnv) (diagnostic.Diagnostic, bool) {
	fact, ok := result.ObjectLiteral(expr)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if arms, ok := closedRecordUnionArms(want); ok {
		if objectLiteralAdmissibleToAnyArm(result, p.resolver, point, arms, fact, env) {
			return diagnostic.Diagnostic{}, false
		}
		extra := objectLiteralUnionArmEvidence(result, p.resolver, point, fact, arms, env)
		return assignmentDiagnostic(name, want, objectLiteralType(want, fact), expr, annotation, extra...), true
	}
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments)
		if !ok {
			continue
		}
		got, readBoundary, ok := objectLiteralEntryMismatchTypeWithSource(result, p.resolver, point, entry, expected, env, factflow.ValueSource{}, false)
		if !ok {
			continue
		}
		memberName := name + segment.FormatSegments(entry.Suffix.Segments)
		if readBoundary == nil {
			readBoundary = boundaryValueFromExpr(entry.Value)
		}
		if topLikeType(got) {
			if declared, declaredOK := dominatingAliasDeclarationType(result, p.resolver, p.flow, point, entry.Value); declaredOK &&
				boundaryProofTypeMismatch(result, point, declared, expected, readBoundary) {
				got = declared
			}
		}
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(entry.Value), exprEvidenceName(entry.Value), expected, readBoundary)
		return objectMemberAssignmentDiagnostic(memberName, expected, got, entry.Value, annotation, extra...), true
	}
	if field, ok := missingRequiredRecordField(want, fact); ok {
		return missingFieldAssignmentDiagnostic(name, want, objectLiteralType(want, fact), field, expr, annotation), true
	}
	if method, ok := missingRequiredInterfaceMethod(want, fact); ok {
		return missingMethodAssignmentDiagnostic(name, want, objectLiteralType(want, fact), method, expr, annotation), true
	}
	return diagnostic.Diagnostic{}, false
}

func dominatingAliasDeclarationType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return nil, false
	}
	got, ok := dominatingRootDeclarationType(result, resolver, flow, point, accessPath.Symbol)
	if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}
