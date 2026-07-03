package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// returnContract reports explicit return annotation mismatches that are already
// proven by the solved return facts.
type returnContract producerContext

func (p returnContract) Produce(result *body.Result) []diagnostic.Diagnostic {
	return produceReturnContract(result, producerContext(p), nil)
}

func produceReturnContract(result *body.Result, context producerContext, inherited map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	fn := result.Function()
	if fn == nil {
		return nil
	}
	producer := returnContract(context)
	returns, ok := lowerReturnTypes(fn, producer.resolver)
	if !ok || len(returns) == 0 {
		return nil
	}
	envs := context.guardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range result.ReturnPoints() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		fact, ok := result.ReturnFact(point)
		if !ok || len(fact.Exprs) == 0 {
			continue
		}
		for i, expr := range fact.Exprs {
			source := returnSourceAt(fact, i)
			want, ok := returnTypeAt(returns, i)
			if !ok || refinement.ContainsFreeTypeParam(want) {
				continue
			}
			annotation := typeExprAt(fn.ReturnTypes, i)
			if annotation == nil {
				continue
			}
			if contextIndependentReturnCallCoveredByRootCallSites(result, context, source, inherited) {
				continue
			}
			if mismatch, ok := objectLiteralMemberMismatch(result, producer.resolver, point, expr, want, envs[point]); ok {
				if returnLocalAssignmentMismatchAlreadyReported(result, producer, envs, mismatch.expr, inherited) {
					continue
				}
				readBoundary := mismatch.readBoundary
				if readBoundary == nil {
					readBoundary = boundaryValueFromExpr(mismatch.expr)
				}
				extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, readBoundary)
				extra = append(extra, mismatch.missingMemberEvidence()...)
				extra = append(extra, mismatch.unionArmEvidence...)
				out = append(out, returnMemberContractDiagnostic(mismatch, annotation, i, extra...))
				continue
			}
			if mismatch, ok := localObjectLiteralReturnMemberMismatch(result, producer.resolver, envs, point, expr, want); ok {
				readBoundary := mismatch.readBoundary
				if readBoundary == nil {
					readBoundary = boundaryValueFromExpr(mismatch.expr)
				}
				extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, readBoundary)
				extra = append(extra, mismatch.missingMemberEvidence()...)
				extra = append(extra, mismatch.unionArmEvidence...)
				out = append(out, returnMemberContractDiagnostic(mismatch, annotation, i, extra...))
				continue
			}
			if castGot, castOK := concreteCastObligationType(result, producer.resolver, point, envs[point], expr); castOK {
				readBoundary := boundaryValueFromASTSource(source)
				if !boundaryTypeMismatch(result, point, castGot, want, readBoundary) {
					continue
				}
				extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(expr), returnContractSubject(i, expr), want, readBoundary)
				out = append(out, returnContractDiagnostic(expr, annotation, castGot, want, i, extra...))
				continue
			}
			got, ok := returnValueType(result, producer.resolver, point, expr, source, inherited)
			if !ok {
				continue
			}
			readBoundary := boundaryValueFromASTSource(source)
			if !boundaryTypeMismatch(result, point, got, want, readBoundary) {
				continue
			}
			if returnLocalAssignmentMismatchAlreadyReported(result, producer, envs, expr, inherited) {
				continue
			}
			extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(expr), returnContractSubject(i, expr), want, readBoundary)
			if optionalIndexReadLacksProof(result, producer.resolver, point, expr) {
				extra = withoutBoundaryMissingProofEvidence(extra)
				if !hasMissingBoundaryProofEvidence(extra) {
					extra = append(extra, missingIndexReadProofEvidence(ast.SpanOf(expr), want))
				}
			} else if !returnContractHasExplicitBoundaryEvidence(extra) {
				extra = withoutBoundaryMissingProofEvidence(extra)
			}
			if len(extra) == 0 {
				extra = explicitTopLikeCastEvidence(ast.SpanOf(expr), want, expr)
			}
			out = append(out, returnContractDiagnostic(expr, annotation, got, want, i, extra...))
		}
	}
	return out
}

func returnLocalAssignmentMismatchAlreadyReported(
	result *body.Result,
	producer returnContract,
	envs map[cfg.Point]guardEnv,
	expr ast.Expr,
	directDefs map[symbol.ID]*ast.FunctionExpr,
) bool {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return false
	}
	ident, ok := inner.(*ast.IdentExpr)
	if !ok {
		return false
	}
	id, ok := result.SymbolOfIdent(ident)
	if !ok || id == 0 {
		return false
	}
	graph := result.Graph()
	if graph == nil {
		return false
	}
	assignability := annotationAssignability(producerContext(producer))
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol || fact.Symbol != id {
			continue
		}
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		_, reported := assignability.localAssignment(result, point, fact, envs[point], directDefs)
		return reported
	}
	return false
}

func localObjectLiteralReturnMemberMismatch(
	result *body.Result,
	resolver typeannotation.Resolver,
	envs map[cfg.Point]guardEnv,
	returnPoint cfg.Point,
	expr ast.Expr,
	want typ.Type,
) (objectLiteralTypeMismatch, bool) {
	ident, castBoundary, ok := returnObjectLiteralIdent(result, expr)
	if !ok {
		return objectLiteralTypeMismatch{}, false
	}
	id, ok := result.SymbolOfIdent(ident)
	if !ok || id == 0 {
		return objectLiteralTypeMismatch{}, false
	}
	graph := result.Graph()
	if graph == nil {
		return objectLiteralTypeMismatch{}, false
	}

	var literalExpr ast.Expr
	var literalPoint cfg.Point
	var literalSidecar factflow.ObjectLiteral
	var hasLiteralSidecar bool
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol || fact.Symbol != id {
			continue
		}
		if literalExpr != nil {
			return objectLiteralTypeMismatch{}, false
		}
		objectFact, ok := result.ObjectLiteral(fact.Expr)
		if !ok || len(objectFact.Entries) == 0 {
			return objectLiteralTypeMismatch{}, false
		}
		literalExpr = fact.Expr
		literalPoint = point
		if lowered, ok := result.LoweredLocalAssignment(point); ok {
			source := lowered.Source()
			if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
				literalSidecar, hasLiteralSidecar = result.ObjectLiteralExpr(source.ExprRef)
			}
		}
	}
	if literalExpr == nil {
		return objectLiteralTypeMismatch{}, false
	}
	if !guardEnvReachableAt(envs, returnPoint) || !guardEnvReachableAt(envs, literalPoint) {
		return objectLiteralTypeMismatch{}, false
	}
	if hasLiteralSidecar {
		if mismatch, ok := objectLiteralMemberMismatchWithValueSources(result, resolver, literalPoint, literalExpr, want, envs[literalPoint], literalSidecar); ok {
			if castBoundary && objectLiteralMismatchIsMissingMember(mismatch) {
				return objectLiteralTypeMismatch{}, false
			}
			return mismatch, true
		}
	}
	mismatch, ok := objectLiteralMemberMismatch(result, resolver, literalPoint, literalExpr, want, envs[literalPoint])
	if castBoundary && objectLiteralMismatchIsMissingMember(mismatch) {
		return objectLiteralTypeMismatch{}, false
	}
	return mismatch, ok
}

func objectLiteralMismatchIsMissingMember(mismatch objectLiteralTypeMismatch) bool {
	return mismatch.missingField != "" || mismatch.missingMethod.Name != ""
}

func returnObjectLiteralIdent(result *body.Result, expr ast.Expr) (*ast.IdentExpr, bool, bool) {
	inner, castBoundary, ok := returnObjectLiteralIdentityInner(expr)
	if !ok {
		return nil, false, false
	}
	if ident, ok := inner.(*ast.IdentExpr); ok {
		return ident, castBoundary, true
	}
	call, ok := inner.(*ast.FuncCallExpr)
	if !ok || call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) == 0 {
		return nil, false, false
	}
	callee, ok := call.Func.(*ast.IdentExpr)
	if !ok || !result.IdentResolvesToGlobal(callee, "setmetatable") {
		return nil, false, false
	}
	arg, argCastBoundary, ok := returnObjectLiteralIdentityInner(call.Args[0])
	if !ok {
		return nil, false, false
	}
	ident, ok := arg.(*ast.IdentExpr)
	return ident, castBoundary || argCastBoundary, ok
}

func returnObjectLiteralIdentityInner(expr ast.Expr) (ast.Expr, bool, bool) {
	castBoundary := false
	for {
		switch wrapped := expr.(type) {
		case nil:
			return nil, castBoundary, false
		case *ast.NonNilAssertExpr:
			if wrapped == nil {
				return nil, castBoundary, false
			}
			expr = wrapped.Expr
		case *ast.CastExpr:
			if wrapped == nil {
				return nil, castBoundary, false
			}
			castBoundary = true
			expr = wrapped.Expr
		default:
			return expr, castBoundary, true
		}
	}
}

func returnValueType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, source sourceprovenance.ASTSource, inherited map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	resolution := firstDiagnosticTypeResolution(
		diagnosticTypeResolution{},
		diagnosticTypeResolutionAttempt{
			Source: "literal",
			Resolve: func() (typ.Type, bool) {
				return valueexpr.LiteralType(expr)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "local-scalar-operator-source",
			Resolve: func() (typ.Type, bool) {
				return localScalarOperatorSourceType(result, resolver, expr)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "explicit-top-like-cast",
			Resolve: func() (typ.Type, bool) {
				return explicitTopLikeCastType(resolver, expr)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "direct-call-return-source",
			Resolve: func() (typ.Type, bool) {
				return directCallReturnSourceType(result, resolver, source, inherited)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "solved-return-source",
			Resolve: func() (typ.Type, bool) {
				return solvedReturnSourceType(result, point, source)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "local-optional-return-source",
			Resolve: func() (typ.Type, bool) {
				return localOptionalReturnSourceType(result, resolver, point, expr, source)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "projected-optional-index",
			Resolve: func() (typ.Type, bool) {
				return projectedOptionalIndexType(result, resolver, point, expr)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "explicit-top-like-expression",
			Resolve: func() (typ.Type, bool) {
				return explicitTopLikeExpressionType(result, resolver, expr)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "explicit-top-like-call-fact-source",
			Resolve: func() (typ.Type, bool) {
				return explicitTopLikeCallFactSourceType(result, resolver, source)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "declared-return-expression",
			Resolve: func() (typ.Type, bool) {
				return declaredReturnExprType(result, resolver, expr)
			},
		},
	)
	return resolution.Type, resolution.OK
}

func contextIndependentReturnCallCoveredByRootCallSites(result *body.Result, context producerContext, source sourceprovenance.ASTSource, inherited map[symbol.ID]*ast.FunctionExpr) bool {
	if result == nil ||
		context.root == nil ||
		context.root == result ||
		context.callContextResult ||
		source.Kind != sourceprovenance.SourceCall ||
		!source.HasCallPoint {
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
	calleeSymbol := site.CalleeSymbol()
	if len(inherited) == 0 || inherited[calleeSymbol] == nil {
		return false
	}
	if localDefs := directCallDefinitions(result, context, nil); localDefs != nil && localDefs[calleeSymbol] != nil {
		return false
	}
	wrapperSymbol, ok := currentFunctionTargetSymbol(result)
	if !ok {
		return false
	}
	rootGraph := context.root.Graph()
	if rootGraph == nil {
		return false
	}
	rootFlow := context.rootFlow
	if rootFlow == nil {
		rootFlow = newDiagnosticFlowCache(context.root)
	}
	seenCall := false
	for _, point := range rootGraph.RPO() {
		rootFact, ok := context.root.Call(point)
		if !ok || rootFact.Call == nil {
			continue
		}
		rootSite, ok := context.root.CallSite(point)
		if !ok || rootSite.CalleeSymbol() != wrapperSymbol {
			continue
		}
		seenCall = true
		if !rootFlow.directFunctionReassignedAfterDefinition(point, calleeSymbol) {
			return false
		}
	}
	return seenCall
}

func currentFunctionTargetSymbol(result *body.Result) (symbol.ID, bool) {
	if result == nil || result.Function() == nil {
		return 0, false
	}
	origin, ok := result.FunctionOrigin(result.Function())
	if !ok || !origin.HasTargetSymbol || origin.TargetSymbol == 0 {
		return 0, false
	}
	return origin.TargetSymbol, true
}

func solvedReturnSourceType(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	got, ok := newDiagnosticQuery(result).SourceType(point, source)
	if !ok ||
		got == nil ||
		typ.IsAny(got) ||
		typ.IsUnknown(got) ||
		typ.IsNever(got) ||
		projectionHasNil(got) ||
		refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func localOptionalReturnSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	p, ok := result.ExpressionPath(expr)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return nil, false
	}
	if declared, ok := declaredPathType(result, resolver, expr); ok && declared != nil && !projectionHasNil(declared) {
		return nil, false
	}
	got, ok := newDiagnosticQuery(result).SourceType(point, source)
	if !ok ||
		got == nil ||
		typ.IsAny(got) ||
		typ.IsUnknown(got) ||
		typ.IsNever(got) ||
		!projectionHasNil(got) ||
		refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func declaredReturnExprType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if result == nil || !staticDotOrIdentExpr(expr) {
		return nil, false
	}
	p, ok := result.ExpressionPath(expr)
	if !ok || p.Symbol == 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(p.Symbol)
	if !ok {
		return nil, false
	}
	root, ok := lowerType(annotation, resolver)
	if !ok || root == nil {
		return nil, false
	}
	root = transparentComparableType(result, root)
	if len(p.Segments) == 0 {
		if !returnContractScalarDeclaredType(root) {
			return nil, false
		}
		return root, true
	}
	got, ok := expectedTypeAtSegments(root, p.Segments)
	if !ok || !returnContractScalarDeclaredType(got) {
		return nil, false
	}
	return got, true
}

func returnContractScalarDeclaredType(t typ.Type) bool {
	if t == nil ||
		typ.IsAny(t) ||
		typ.IsUnknown(t) ||
		typ.IsNever(t) ||
		projectionHasNil(t) ||
		refinement.ContainsFreeTypeParam(t) {
		return false
	}
	switch t.Kind() {
	case kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
		return true
	default:
		return false
	}
}

func staticDotOrIdentExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e != nil
	case *ast.AttrGetExpr:
		return e != nil && e.KeySyntax == ast.AttrKeyDot && staticDotOrIdentExpr(e.Object)
	default:
		return false
	}
}

func directCallReturnSourceType(result *body.Result, resolver typeannotation.Resolver, source sourceprovenance.ASTSource, inherited map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	return directCallReturnSourceTypeWithContext(result, producerContext{resolver: resolver, flow: newDiagnosticFlowCache(result)}, source, inherited)
}

func directCallReturnSourceTypeWithContext(result *body.Result, context producerContext, source sourceprovenance.ASTSource, inherited map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return nil, false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return nil, false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok || site.CalleeSymbol() == 0 {
		return nil, false
	}
	contract, _, ok := directCallResultContract(result, context, source.CallPoint, fact, site, inherited[site.CalleeSymbol()], inherited)
	if !ok {
		return nil, false
	}
	if got, ok := contract.returnType(source.ResultIndex); ok {
		if refinement.ContainsFreeTypeParam(got) {
			return nil, false
		}
		return got, true
	}
	got, ok := contract.declaredReturnType(source.ResultIndex)
	if !ok || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func returnSourceAt(fact semantics.ReturnFact, index int) sourceprovenance.ASTSource {
	if index >= 0 && index < len(fact.Sources) {
		return fact.Sources[index]
	}
	return sourceprovenance.NewUnknownSource(sourceprovenance.NoSourceIndex)
}

func lowerReturnTypes(fn *ast.FunctionExpr, resolver typeannotation.Resolver) ([]directCallResult, bool) {
	if fn == nil {
		return nil, false
	}
	returns := make([]directCallResult, 0, len(fn.ReturnTypes))
	for _, retExpr := range fn.ReturnTypes {
		ret, ok := lowerDirectCallResult(retExpr, resolver)
		if !ok {
			return nil, false
		}
		returns = append(returns, ret)
	}
	return returns, true
}

func returnTypeAt(returns []directCallResult, index int) (typ.Type, bool) {
	if index < 0 || index >= len(returns) {
		return nil, false
	}
	ret := returns[index]
	if !ret.explicit || ret.typ == nil || typ.IsAny(ret.typ) || typ.IsUnknown(ret.typ) {
		return nil, false
	}
	return ret.typ, true
}

func returnContractDiagnostic(expr ast.Expr, annotation ast.TypeExpr, got, want typ.Type, index int, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	return returnContractDiagnosticWithLabel(expr, annotation, got, want, index, "", extraEvidence...)
}

func returnMemberContractDiagnostic(mismatch objectLiteralTypeMismatch, annotation ast.TypeExpr, index int, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	label := ""
	if mismatch.suffix != "" {
		if index >= 0 {
			label = fmt.Sprintf("returned value %d%s", index+1, mismatch.suffix)
		} else {
			label = "returned value" + mismatch.suffix
		}
	}
	return returnContractDiagnosticWithLabel(mismatch.expr, annotation, mismatch.got, mismatch.want, index, label, extraEvidence...)
}

func returnContractDiagnosticWithLabel(expr ast.Expr, annotation ast.TypeExpr, got, want typ.Type, index int, labelOverride string, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	exprName := exprEvidenceNameOK(expr)
	exprSpan := spanWithEvidenceName(ast.SpanOf(expr), exprName)
	typeSpan := ast.SpanOf(annotation)
	label := "returned value"
	if index >= 0 {
		label = fmt.Sprintf("returned value %d", index+1)
	}
	if labelOverride != "" {
		label = labelOverride
	}
	subject := label
	if exprName != "" {
		subject = fmt.Sprintf("%s (%s)", label, exprName)
	}
	extraEvidence = clarifyReturnContractEvidence(extraEvidence, subject, exprSpan)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    exprSpan,
			Message: assignmentSourceTypeEvidence(subject, got),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    typeSpan,
			Message: returnDeclaredTypeEvidence(label, want),
		},
	}
	evidence = append(evidence, extraEvidence...)
	message := returnContractMessage(label, expr, got, want)
	if returnContractHasExplicitBoundaryEvidence(extraEvidence) && !nilSafetyMismatch(got, want) {
		message = display.ReturnBoundaryProofMessage(label, expr, want)
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        exprSpan,
		Code:        CodeReturnContractType,
		Severity:    diagnostic.SeverityError,
		Message:     message,
		Help:        returnContractHelp(exprName, got),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels: []diagnostic.Label{
			sourceLabel(exprSpan, labelReturnedValue),
			sourceLabel(typeSpan, labelDeclaredReturn),
		},
	})
}

func returnContractHasExplicitBoundaryEvidence(items []diagnostic.Evidence) bool {
	for _, item := range items {
		if item.Reason == diagnostic.EvidenceReasonExplicitBoundaryValidation {
			return true
		}
	}
	return false
}

func withoutBoundaryMissingProofEvidence(items []diagnostic.Evidence) []diagnostic.Evidence {
	out := items[:0]
	for _, item := range items {
		if item.Kind == diagnostic.EvidenceMissingProof && item.Reason == diagnostic.EvidenceReasonBoundaryValidationMissing {
			continue
		}
		out = append(out, item)
	}
	return out
}

func returnContractSubject(index int, expr ast.Expr) string {
	label := "returned value"
	if index >= 0 {
		label = fmt.Sprintf("returned value %d", index+1)
	}
	if exprName := exprEvidenceNameOK(expr); exprName != "" {
		return fmt.Sprintf("%s (%s)", label, exprName)
	}
	return label
}

func clarifyReturnContractEvidence(items []diagnostic.Evidence, subject string, subjectSpan diagnostic.Span) []diagnostic.Evidence {
	if len(items) == 0 || subject == "" {
		return items
	}
	out := append([]diagnostic.Evidence(nil), items...)
	for i := range out {
		if subjectSpan.Valid() && sameStart(out[i].Span, subjectSpan) && !hasUsefulEnd(out[i].Span) {
			out[i].Span = subjectSpan
		}
		switch out[i].Reason {
		case diagnostic.EvidenceReasonIndexReadValidationMissing:
			out[i].Message = returnIndexedReadProofMessage(subject)
		case diagnostic.EvidenceReasonExplicitBoundaryValidation:
			out[i].Message = returnExplicitBoundaryProofMessage(subject)
		case diagnostic.EvidenceReasonBoundaryValidationMissing:
			out[i].Message = returnMissingProofMessage(subject)
		}
	}
	return out
}

// directCallResultAssignment reports mismatches between direct-call return
// contracts and annotated local targets that receive those call results.
type directCallResultAssignment producerContext

func (p directCallResultAssignment) Produce(result *body.Result) []diagnostic.Diagnostic {
	return produceDirectCallResultAssignment(result, producerContext(p), nil)
}

func produceDirectCallResultAssignment(result *body.Result, context producerContext, inherited map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	defs := directCallDefinitions(result, context, inherited)
	producer := directCallResultAssignment(context)
	envs := context.guardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok {
			continue
		}
		contract, name, ok := directCallResultContract(result, producerContext(producer), point, fact, site, defs[site.CalleeSymbol()], defs)
		if !ok {
			continue
		}
		for _, target := range fact.ResultTargets {
			if target.Kind != semantics.CallResultTargetLocalAssignment || !target.HasSymbol || target.Symbol == 0 {
				continue
			}
			wantExpr, ok := result.SymbolTypeAnnotation(target.Symbol)
			if !ok {
				continue
			}
			want, ok := lowerType(wantExpr, producer.resolver)
			if !ok || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
				continue
			}
			resultIndex := target.ResultIndex
			if callResultBoundarySatisfiesTarget(result, point, resultIndex, want) {
				continue
			}
			ret, ok := contract.returnResult(resultIndex)
			if !ok {
				continue
			}
			got, ok := ret.returnType()
			if !ok {
				got, ok = ret.declaredReturnType()
			}
			if !ok || refinement.ContainsFreeTypeParam(got) {
				continue
			}
			readBoundary := boundaryCallResultReader(point, resultIndex)
			untrustedTopLike := topLikeType(got)
			if untrustedTopLike && name == "require" {
				continue
			}
			if untrustedTopLike {
				readBoundary = untrustedAnyBoundaryReader(readBoundary)
			}
			if !boundaryTypeMismatch(result, point, got, want, readBoundary) {
				continue
			}
			var extra []diagnostic.Evidence
			if untrustedTopLike {
				extra = boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(fact.Call), callResultSubject(resultIndex), want, readBoundary)
			}
			out = append(out, directCallResultAssignmentDiagnostic(point, fact.Call, name, result.SymbolName(target.Symbol), resultIndex, ret, got, want, wantExpr, extra...))
		}
	}
	return out
}

func callResultBoundarySatisfiesTarget(result *body.Result, point cfg.Point, resultIndex int, want typ.Type) bool {
	if result == nil || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	reader := solvedBoundaryCallResultReader(point, resultIndex)
	value, ok := reader(result, point)
	if !ok {
		return false
	}
	got, ok := newDiagnosticQuery(result).ValueTypeWithPresence(value)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) || refinement.ContainsFreeTypeParam(got) {
		return false
	}
	return !boundaryTypeMismatch(result, point, got, want, reader)
}

func boundaryCallResultReader(callPoint cfg.Point, resultIndex int) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		if resultIndex < 0 {
			return product.Value{}, false
		}
		source, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, resultIndex, callPoint, factflow.ValueSourceShape{})
		if !ok {
			return product.Value{}, false
		}
		return newDiagnosticQuery(result).ValueSourceForExplanationAtBoundary(point, source)
	}
}

func solvedBoundaryCallResultReader(callPoint cfg.Point, resultIndex int) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		if result == nil || resultIndex < 0 {
			return product.Value{}, false
		}
		source, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, resultIndex, callPoint, factflow.ValueSourceShape{})
		if !ok {
			return product.Value{}, false
		}
		return newDiagnosticQuery(result).SourceValueAtBoundary(point, source)
	}
}

func directCallResultContract(result *body.Result, context producerContext, point cfg.Point, fact semantics.CallFact, site factflow.CallSite, def *ast.FunctionExpr, defs map[symbol.ID]*ast.FunctionExpr) (directFunctionContract, string, bool) {
	name := directCallDisplayName(result, site)
	if contract, ok := currentDirectFunctionContract(result, context, point, fact, site, name, def); ok {
		var violations []typecall.ArgumentConstraintViolation
		contract, violations = instantiateDirectFunctionContract(result, point, fact, contract, context, defs)
		if len(violations) > 0 {
			return directFunctionContract{}, "", false
		}
		if !directCallArgsCompatible(result, point, fact, contract, context, defs) {
			return directFunctionContract{}, "", false
		}
		return contract, name, true
	}
	if def != nil {
		contract, ok := lowerDirectFunctionContractInResultScope(result, def, context.resolver)
		if !ok {
			return directFunctionContract{}, "", false
		}
		contract.name = name
		contract.declSpan = ast.SpanOf(def)
		var violations []typecall.ArgumentConstraintViolation
		contract, violations = instantiateDirectFunctionContract(result, point, fact, contract, context, defs)
		if len(violations) > 0 {
			return directFunctionContract{}, "", false
		}
		if !directCallArgsCompatible(result, point, fact, contract, context, defs) {
			return directFunctionContract{}, "", false
		}
		return contract, name, true
	}
	if fn, ok := result.CallSignatureType(site); ok {
		contract := lowerDirectFunctionType(fn)
		contract.name = name
		contract.declSpan = ast.SpanOf(fact.Call)
		var violations []typecall.ArgumentConstraintViolation
		contract, violations = instantiateDirectFunctionContract(result, point, fact, contract, context, defs)
		if len(violations) > 0 {
			return directFunctionContract{}, "", false
		}
		if !directCallArgsCompatible(result, point, fact, contract, context, defs) {
			return directFunctionContract{}, "", false
		}
		return contract, name, true
	}
	if fact.Call != nil && fact.Call.Func != nil {
		if calleeType, ok := directCallCalleeType(result, context, point, fact.Call.Func); ok {
			if callable, ok := typecall.Callable(calleeType); ok && callable != nil {
				contract := lowerDirectFunctionType(callable)
				if callPath := site.CalleePathRef(); !callPath.IsEmpty() {
					name = displayPath(result, callPath)
				}
				contract.name = name
				contract.declSpan = ast.SpanOf(fact.Call.Func)
				var violations []typecall.ArgumentConstraintViolation
				contract, violations = instantiateDirectFunctionContract(result, point, fact, contract, context, defs)
				if len(violations) > 0 {
					return directFunctionContract{}, "", false
				}
				if !directCallArgsCompatible(result, point, fact, contract, context, defs) {
					return directFunctionContract{}, "", false
				}
				return contract, name, true
			}
		}
	}
	baseExpr, ok := result.SymbolTypeAnnotation(site.CalleeSymbol())
	if !ok {
		return directFunctionContract{}, "", false
	}
	baseType, ok := lowerType(baseExpr, context.resolver)
	if !ok || typ.IsAny(baseType) || typ.IsUnknown(baseType) {
		return directFunctionContract{}, "", false
	}
	callable, ok := typecall.Callable(baseType)
	if !ok {
		return directFunctionContract{}, "", false
	}
	contract := lowerDirectFunctionType(callable)
	contract.name = name
	contract.declSpan = ast.SpanOf(fact.Call)
	var violations []typecall.ArgumentConstraintViolation
	contract, violations = instantiateDirectFunctionContract(result, point, fact, contract, context, defs)
	if len(violations) > 0 {
		return directFunctionContract{}, "", false
	}
	if !directCallArgsCompatible(result, point, fact, contract, context, defs) {
		return directFunctionContract{}, "", false
	}
	return contract, name, true
}

func directCallCalleeType(result *body.Result, context producerContext, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if got, ok := explicitAnnotatedCalleeType(result, context.resolver, expr); ok {
		return got, true
	}
	if got, ok := declaredPathType(result, context.resolver, expr); ok && got != nil && !typ.IsAny(got) && !typ.IsUnknown(got) {
		return got, true
	}
	env := context.guardEnv(result, point)
	if got, ok := newFlowExpressionTyper(result, context.resolver, point, env).typeOf(expr); ok {
		return got, true
	}
	if value, ok := newDiagnosticQuery(result).ExpressionValueAtBoundary(point, expr); ok {
		if got, ok := typevalue.TypeOf(result.Registry(), value); ok {
			return got, true
		}
	}
	return staticExpressionType(result, context.resolver, expr)
}

func explicitAnnotatedCalleeType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	if _, ok := expr.(*ast.IdentExpr); !ok {
		return nil, false
	}
	calleePath, ok := result.ExpressionPath(expr)
	if !ok || calleePath.Symbol == 0 || len(calleePath.Segments) != 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(calleePath.Symbol)
	if !ok {
		return nil, false
	}
	got, ok := lowerType(annotation, resolver)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) {
		return nil, false
	}
	return got, true
}

func directCallArgsCompatible(result *body.Result, point cfg.Point, fact semantics.CallFact, contract directFunctionContract, context producerContext, defs map[symbol.ID]*ast.FunctionExpr) bool {
	if fact.Call == nil {
		return false
	}
	env := context.guardEnv(result, point)
	args := fact.Call.Args
	required := contract.requiredArity()
	if len(args) < required {
		return false
	}
	if !contract.hasVararg && len(args) > len(contract.params) {
		return false
	}
	contract, violations := instantiateDirectFunctionContract(result, point, fact, contract, context, defs)
	if len(violations) > 0 {
		return false
	}
	for i, arg := range args {
		var want typ.Type
		if i < len(contract.params) {
			param := contract.params[i]
			want = param.typ
			if !param.explicit || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
				continue
			}
		} else if contract.hasVararg {
			want = contract.variadic.typ
			if !contract.variadic.explicit || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
				continue
			}
		} else {
			break
		}
		resolution := resolveDirectCallCompatibleArgumentType(result, point, fact, i, arg, context, defs)
		if !resolution.OK || refinement.ContainsFreeTypeParam(want) || refinement.ContainsFreeTypeParam(resolution.Type) {
			continue
		}
		if _, ok := objectLiteralMemberMismatch(result, context.resolver, point, arg, want, env); ok {
			return false
		}
		got := resolution.Type
		readBoundary := boundaryCallArgumentReader(fact, i, arg)
		if topLikeType(got) {
			readBoundary = untrustedAnyBoundaryReader(readBoundary)
		}
		if resolution.UntrustedTopLike {
			if boundaryProofTypeMismatch(result, point, got, want, readBoundary) {
				return false
			}
			continue
		}
		if directCallArgumentTypeMismatch(result, point, got, want, readBoundary) {
			return false
		}
	}
	return true
}

func resolveDirectCallCompatibleArgumentType(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	index int,
	arg ast.Expr,
	context producerContext,
	defs map[symbol.ID]*ast.FunctionExpr,
) diagnosticTypeResolution {
	return firstDiagnosticTypeResolution(
		diagnosticTypeResolution{},
		diagnosticTypeResolutionAttempt{
			Source: "declared-argument-expression",
			Resolve: func() (typ.Type, bool) {
				got, ok := declaredArgumentExprType(result, context.resolver, arg)
				if ok && topLikeType(got) {
					return nil, false
				}
				return got, ok
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "projected-structural-flow-source",
			Resolve: func() (typ.Type, bool) {
				got, ok := projectedStructuralFlowSourceType(result, context.resolver, point, guardEnv{}, arg)
				if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
					return nil, false
				}
				return got, true
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "flow-expression",
			Resolve: func() (typ.Type, bool) {
				got, ok := newFlowExpressionTyper(result, context.resolver, point, guardEnv{}).typeOf(arg)
				if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
					return nil, false
				}
				return got, true
			},
		},
		diagnosticTypeResolutionAttempt{
			Source:           "untrusted-top-like-expression",
			UntrustedTopLike: true,
			Resolve: func() (typ.Type, bool) {
				return untrustedTopLikeExpressionTypeAt(result, context.resolver, point, arg)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "direct-call-argument-contract-source",
			Resolve: func() (typ.Type, bool) {
				return directCallArgumentContractSourceType(result, context, fact, index, defs)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "static-expression",
			Resolve: func() (typ.Type, bool) {
				return staticExpressionType(result, context.resolver, arg)
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "boundary-call-argument-source",
			Resolve: func() (typ.Type, bool) {
				return boundaryCallArgumentSourceType(result, point, fact, index)
			},
		},
	)
}

func instantiateDirectFunctionContract(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	contract directFunctionContract,
	context producerContext,
	defs map[symbol.ID]*ast.FunctionExpr,
) (directFunctionContract, []typecall.ArgumentConstraintViolation) {
	if contract.source == nil || len(contract.source.TypeParams) == 0 || fact.Call == nil {
		return contract, nil
	}
	args := directCallArgumentTypes(result, context, point, fact, defs)
	fn, violations, trace := typecall.InstantiateGenericCallWithTrace(contract.source, args)
	if fn == nil || fn == contract.source {
		return contract, violations
	}
	instantiated := lowerDirectFunctionType(fn)
	instantiated.name = contract.name
	instantiated.declSpan = contract.declSpan
	instantiated.genericTrace = trace
	return instantiated, violations
}

func directCallArgumentTypes(result *body.Result, context producerContext, point cfg.Point, fact semantics.CallFact, defs map[symbol.ID]*ast.FunctionExpr) []typ.Type {
	if fact.Call == nil {
		return nil
	}
	site, hasSite := result.CallSite(point)
	args := make([]typ.Type, len(fact.Call.Args))
	for i, arg := range fact.Call.Args {
		got, ok := directObjectLiteralArgumentType(result, context.resolver, point, arg, defs)
		if !ok {
			got, ok = directCallArgumentFlowExpressionType(result, context.resolver, point, guardEnv{}, arg)
		}
		if !ok {
			got, ok = declaredArgumentExprType(result, context.resolver, arg)
			if ok && topLikeType(got) {
				ok = false
			}
		}
		if !ok {
			got, ok = directCallArgumentContractSourceType(result, context, fact, i, defs)
		}
		if !ok {
			got, ok = staticExpressionType(result, context.resolver, arg)
		}
		if !ok {
			got, ok = boundaryCallArgumentSourceType(result, point, fact, i)
		}
		if !ok && hasSite {
			got, ok = boundaryCallArgumentValueSourceType(result, point, site, i)
		}
		if ok {
			args[i] = got
		}
	}
	return args
}

func boundaryCallArgumentValueSourceType(result *body.Result, point cfg.Point, site factflow.CallSite, index int) (typ.Type, bool) {
	source, ok := site.ArgumentSourceAt(index)
	if !ok {
		return nil, false
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ValueSourceForExplanationAtBoundary(point, source)
	if !ok {
		return nil, false
	}
	return query.ValueType(value)
}

func directObjectLiteralArgumentType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, defs map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return nil, false
	}
	table, ok := inner.(*ast.TableExpr)
	if !ok {
		return nil, false
	}
	builder := typetable.NewRecord()
	seen := false
	for _, field := range table.Fields {
		name, syntax, ok := directObjectLiteralFieldKey(field)
		if !ok || name == "" || field.Value == nil {
			continue
		}
		t, ok := directObjectLiteralFieldType(result, resolver, point, field.Value, defs)
		if !ok || t == nil {
			continue
		}
		switch syntax {
		case ast.AttrKeyIndex:
			builder.StaticStringIndex(name, t)
		default:
			builder.Field(name, t)
		}
		seen = true
	}
	if !seen {
		return nil, false
	}
	return builder.Build(), true
}

func directObjectLiteralFieldKey(field *ast.Field) (string, ast.AttrKeySyntax, bool) {
	if field == nil || field.Key == nil {
		return "", ast.AttrKeyUnknown, false
	}
	switch key := field.Key.(type) {
	case *ast.StringExpr:
		return key.Value, field.KeySyntax, key.Value != ""
	case *ast.IdentExpr:
		return key.Value, field.KeySyntax, key.Value != ""
	default:
		return "", ast.AttrKeyUnknown, false
	}
}

func directObjectLiteralFieldType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, defs map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	if t, ok := directFunctionValueType(result, resolver, point, expr, defs); ok {
		return t, true
	}
	if t, ok := staticExpressionType(result, resolver, expr); ok {
		return t, true
	}
	if t, ok := directObjectLiteralArgumentType(result, resolver, point, expr, defs); ok {
		return t, true
	}
	return nil, false
}

func directFunctionValueType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, defs map[symbol.ID]*ast.FunctionExpr) (typ.Type, bool) {
	if result != nil {
		if fn, ok := result.FunctionValueTypeAtBoundary(point, expr); ok && fn != nil {
			return fn, true
		}
	}
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		return lowerFunctionExprType(fn, resolver)
	}
	if result == nil || len(defs) == 0 || expr == nil {
		return nil, false
	}
	p, ok := result.ExpressionPath(expr)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return nil, false
	}
	def, ok := defs[p.Symbol]
	if !ok || def == nil {
		return nil, false
	}
	return lowerFunctionExprType(def, resolver)
}

func directCallResultAssignmentDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, targetName string, index int, ret directCallResult, got, want typ.Type, annotation ast.TypeExpr, extra ...diagnostic.Evidence) diagnostic.Diagnostic {
	callSpan := ast.SpanOf(call)
	typeSpan := ast.SpanOf(annotation)
	label := callResultSubject(index)
	evidence := make([]diagnostic.Evidence, 0, len(extra)+4)
	if ret.declSpan.Valid() {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    ret.declSpan,
			Message: callResultDeclaredReturnEvidence(name, label, got),
		})
	} else {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    callSpan,
			Message: fmt.Sprintf("%s returns %s", name, formatType(got)),
		})
	}
	evidence = append(evidence, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceUserAssertion,
		Trust:   diagnostic.TrustClaimed,
		Span:    typeSpan,
		Message: assignmentTargetTypeEvidence(targetName, want),
	})
	if nilSafetyMismatch(got, want) {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    callSpan,
			Message: callResultMissingNonNilProofMessage(label),
		})
	}
	evidence = append(evidence, extra...)
	labels := []diagnostic.Label{
		sourceLabel(callSpan, labelCallResult),
		sourceLabel(typeSpan, labelDeclaredType),
	}
	if ret.declSpan.Valid() {
		declLabel := ret.declLabel
		if declLabel == "" {
			declLabel = labelDeclaredReturn
		}
		labels = append(labels, sourceLabel(ret.declSpan, declLabel))
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        callSpan,
		Code:        CodeDirectCallResultAssignment,
		Severity:    diagnostic.SeverityError,
		Message:     fmt.Sprintf("%s is %s, not %s", label, formatType(got), formatType(want)),
		Help:        callResultAssignmentHelp(got),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels:      labels,
	})
}

func callResultSubject(index int) string {
	if index >= 0 {
		return fmt.Sprintf("call result %d", index+1)
	}
	return "call result"
}
