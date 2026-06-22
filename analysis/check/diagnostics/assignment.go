package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
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
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
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
	got, ok := readmodel.New(result).ValueTypeWithPresence(value)
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
	return underSuppliedTargetDiagnostic(fact.Name, want, got, fact.Type), true
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
	if directCallResultAssignmentWouldReport(result, producerContext(p), fact.Source, want, directDefs) {
		return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
	}
	if directCallContractWouldReport(result, producerContext(p), fact.Source, directDefs, env) {
		return diagnostic.Diagnostic{}, false
	}
	if directCallResultOwner(result, fact.Source) {
		if got, ok := directFunctionCurrentReturnPathType(result, p.resolver, fact.Source, directDefs); ok {
			if boundaryTypeMismatch(result, fact.Source.CallPoint, got, want, nil) {
				return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
			}
		}
		if got, ok := callResultWitnessProvenMismatchType(result, point, fact.Source, want); ok {
			return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
		}
	}
	if directCallResultOwner(result, fact.Source) && !directCallSourceHasTypedSignature(result, fact.Source) {
		if !callResultWitnessProvenMismatch(result, point, fact.Source, want) {
			return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
		}
	}
	sourceResolution := resolveLocalAssignmentSourceType(result, p.resolver, p.flow, point, fact, env, directDefs)
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
	if mismatch {
		if callResultFieldOptionalImprecision(result, p.flow, point, fact.Expr, got, want) {
			return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
		}
		if expressionHasMissingMemberRead(result, p.resolver, point, env, fact.Expr) {
			return diagnostic.Diagnostic{}, false
		}
		var extra []diagnostic.Evidence
		if sourceResolution.ReassignedCallResultProjection {
			extra = append(extra, reassignedCallResultFieldEvidence(result, p.resolver, p.flow, point, fact.Expr)...)
		}
		extra = append(extra, optionalReceiverCauseEvidence(result, p.resolver, point, env, fact.Expr)...)
		extra = append(extra, callInvalidatedPathEvidence(result, point, fact.Expr)...)
		extra = append(extra, boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(fact.Expr), exprEvidenceName(fact.Expr), want, readBoundary)...)
		if sourceResolution.OptionalIndexProjection && !hasMissingBoundaryProofEvidence(extra) {
			extra = append(extra, missingIndexReadProofEvidence(ast.SpanOf(fact.Expr), want))
		}
		return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type, extra...), true
	}
	return p.objectLiteralAssignment(result, point, fact.Name, want, fact.Expr, fact.Type, env)
}

type localAssignmentSourceTypeResolution struct {
	Type                           typ.Type
	CastInnerExpr                  ast.Expr
	OK                             bool
	OptionalIndexProjection        bool
	PresenceAwareSourceProjection  bool
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
	if r.DeclarationProjection || r.OptionalIndexProjection {
		return nil
	}
	return readBoundary
}

func resolveLocalAssignmentSourceType(
	result *body.Result,
	resolver typeannotation.Resolver,
	flow *diagnosticFlowCache,
	point cfg.Point,
	fact semantics.LocalAssignmentFact,
	env guardEnv,
	directDefs map[symbol.ID]*ast.FunctionExpr,
) localAssignmentSourceTypeResolution {
	if got, ok := valueexpr.LiteralType(fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := projectedOptionalIndexType(result, resolver, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, OptionalIndexProjection: true}
	}
	if got, ok := concreteCastObligationType(result, resolver, point, env, fact.Expr); ok {
		out := localAssignmentSourceTypeResolution{Type: got, OK: true, UntrustedTopLike: true}
		if inner, innerOK := concreteCastInner(fact.Expr); innerOK {
			out.CastInnerExpr = inner
		}
		return out
	}
	if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, UntrustedTopLike: true}
	}
	if got, ok := untrustedAnnotatedIdentifierType(result, resolver, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, UntrustedTopLike: true}
	}
	if got, ok := result.FunctionValueTypeAtBoundary(point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := projectedFlowSourceType(result, resolver, point, env, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := freshRecordAbsentFieldSourceType(result, resolver, point, env, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := dominatingCallResultPathType(result, resolver, flow, point, fact.Expr, directDefs); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := dominatingCallResultFieldSourceType(result, flow, point, fact.Expr, fact.Source); ok {
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
	if got, ok := sourceExpressionTypeWithPresence(result, point, fact.Source); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, PresenceAwareSourceProjection: true}
	}
	if got, ok := readmodel.New(result).SourceType(point, fact.Source); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := dominatingDeclarationProjectionType(result, resolver, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true, DeclarationProjection: true}
	}
	if got, ok := localScalarOperatorSourceType(result, resolver, fact.Expr); ok {
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
	if got, ok := optionalMemberReadType(result, resolver, point, env, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	if got, ok := callInvalidatedBoundaryExprType(result, resolver, point, fact.Expr); ok {
		return localAssignmentSourceTypeResolution{Type: got, OK: true}
	}
	return localAssignmentSourceTypeResolution{}
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
	if mismatch, ok := objectLiteralMemberMismatch(result, point, fact.Value, want, env); ok {
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
		extra = append(extra, mismatch.missingFieldEvidence()...)
		return pathAssignmentDiagnostic(fact.Target, mismatch.expr, mismatch.got, mismatch.want, extra...), true
	}
	sourceResolution := resolvePathAssignmentSourceType(result, p.resolver, point, fact, env)
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
	resolver typeannotation.Resolver,
	point cfg.Point,
	fact semantics.OrdinaryAssignmentFact,
	env guardEnv,
) pathAssignmentSourceTypeResolution {
	if got, ok := concreteCastObligationType(result, resolver, point, env, fact.Value); ok {
		out := pathAssignmentSourceTypeResolution{Type: got, OK: true, UntrustedTopLike: true}
		if inner, innerOK := concreteCastInner(fact.Value); innerOK {
			out.CastInnerExpr = inner
		}
		return out
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

func (p annotationAssignability) optionalAssignmentTarget(result *body.Result, point cfg.Point, target ast.Expr, env guardEnv) (diagnostic.Diagnostic, bool) {
	attr, ok := assignmentTargetAttr(target)
	if !ok || attr.Object == nil {
		return diagnostic.Diagnostic{}, false
	}
	receiverType, ok := newFlowExpressionTyper(result, p.resolver, point, env).typeOf(attr.Object)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	if !projectionHasNil(receiverType) {
		return diagnostic.Diagnostic{}, false
	}
	return optionalAssignmentTargetDiagnostic(attr.Object, target, receiverType), true
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
	var out []diagnostic.Diagnostic
	memberRead(producerContext{resolver: resolver}).walk(expr, typers, make(map[*ast.AttrGetExpr]struct{}), &out)
	return len(out) != 0
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

func directCallResultAssignmentWouldReport(result *body.Result, context producerContext, source sourceprovenance.ASTSource, want typ.Type, defs map[symbol.ID]*ast.FunctionExpr) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint ||
		want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
		return false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok {
		return false
	}
	var def *ast.FunctionExpr
	if site.CalleeSymbol() != 0 {
		def = defs[site.CalleeSymbol()]
	}
	contract, _, ok := directCallResultContract(result, context, source.CallPoint, fact, site, def, defs)
	if !ok {
		return false
	}
	got, ok := contract.returnType(source.ResultIndex)
	if !ok {
		got, ok = contract.declaredReturnType(source.ResultIndex)
	}
	if !ok || refinement.ContainsFreeTypeParam(got) {
		return false
	}
	return boundaryTypeMismatch(result, source.CallPoint, got, want, boundaryCallResultReader(source.CallPoint, source.ResultIndex))
}

func directCallContractWouldReport(result *body.Result, context producerContext, source sourceprovenance.ASTSource, defs map[symbol.ID]*ast.FunctionExpr, env guardEnv) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok || site.CalleeSymbol() == 0 {
		return false
	}
	producer := directCallContract(context)
	if directCallSiteUsesMemberAccess(site, fact) {
		if !hasTypedCallSignature(result, site) {
			if _, ok := memberCall(context).call(result, source.CallPoint, fact, env); ok {
				return true
			}
			if contract, ok := currentDirectFunctionContract(result, context, source.CallPoint, fact, directCallDisplayName(result, site), defs[site.CalleeSymbol()]); ok {
				_, ok := producer.directFunctionCall(result, source.CallPoint, fact, contract, defs, env)
				return ok
			}
			return false
		}
		if _, ok := memberCall(context).typedSignatureStructuralDiagnostic(result, source.CallPoint, fact, env); ok {
			return true
		}
	}
	_, ok = producer.call(result, source.CallPoint, fact, site, defs[site.CalleeSymbol()], defs, env)
	return ok
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
	reader := readmodel.New(result)
	value, ok := reader.SourceValue(point, source)
	if !ok {
		return nil, false
	}
	if !reader.ValueWitnessProvenMismatch(value, want) {
		return nil, false
	}
	got, ok := reader.ValueType(value)
	return got, ok
}

func directCallSourceHasTypedSignature(result *body.Result, source sourceprovenance.ASTSource) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok {
		return false
	}
	return hasTypedCallSignature(result, site)
}

func (p annotationAssignability) objectLiteralAssignment(result *body.Result, point cfg.Point, name string, want typ.Type, expr ast.Expr, annotation ast.TypeExpr, env guardEnv) (diagnostic.Diagnostic, bool) {
	fact, ok := result.ObjectLiteral(expr)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if arms, ok := closedRecordUnionArms(want); ok {
		if objectLiteralAdmissibleToAnyArm(result, point, arms, fact, env) {
			return diagnostic.Diagnostic{}, false
		}
		return assignmentDiagnostic(name, want, objectLiteralType(want, fact), expr, annotation), true
	}
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments)
		if !ok {
			continue
		}
		got, ok := objectLiteralEntryMismatchType(result, point, entry, expected, env)
		if !ok {
			continue
		}
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(entry.Value), exprEvidenceName(entry.Value), expected, boundaryValueFromExpr(entry.Value))
		return assignmentDiagnostic(name+segment.FormatSegments(entry.Suffix.Segments), expected, got, entry.Value, annotation, extra...), true
	}
	if field, ok := missingRequiredRecordField(want, fact); ok {
		return missingFieldAssignmentDiagnostic(name, want, objectLiteralType(want, fact), field, expr, annotation), true
	}
	return diagnostic.Diagnostic{}, false
}
