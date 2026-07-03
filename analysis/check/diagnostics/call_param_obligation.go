package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// callParamObligations reports call-site argument mismatches from pre-call
// obligations projected through summaries.
type callParamObligations producerContext

func (p callParamObligations) Produce(result *body.Result) []diagnostic.Diagnostic {
	return produceCallParamObligations(result, producerContext(p), nil)
}

func produceCallParamObligations(
	result *body.Result,
	context producerContext,
	inherited map[symbol.ID]*ast.FunctionExpr,
) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := context.guardEnvironments(result)
	producer := callParamObligations(context)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		site, _ := result.CallSite(point)
		outcome, ok := result.CallOutcomeAt(point)
		if !ok || len(outcome.ParamObligations) == 0 {
			continue
		}
		if d, ok := producer.call(result, point, fact, site, outcome.ParamObligations, inherited, envs[point]); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p callParamObligations) call(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	obligations []callpayload.CallParamObligation,
	inherited map[symbol.ID]*ast.FunctionExpr,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	args := fact.Call.Args
	if memberCallContractWouldReport(result, point, fact, site, producerContext(p), env) ||
		directMemberCallContractWouldReport(result, point, fact, site, producerContext(p), env) {
		return diagnostic.Diagnostic{}, false
	}
	seen := make(map[int]struct{}, len(obligations))
	for _, obligation := range obligations {
		i := obligation.ParamIndex
		argIndex := i
		if argIndex < 0 || argIndex >= len(args) {
			continue
		}
		if _, ok := seen[argIndex]; ok {
			continue
		}
		want, ok := newDiagnosticQuery(result).ValueType(obligation.Value)
		if !ok || want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		if !obligation.Origin.HasOrigin {
			if richer, ok := originObligationForParam(result, obligations, i, want); ok {
				obligation = richer
			}
		}
		if explicitContractParamCoversObligation(result, point, fact, site, i, want, p.resolver, inherited) {
			continue
		}
		seen[argIndex] = struct{}{}
		if mismatch, ok := objectLiteralMemberMismatchForCallArgument(result, p.resolver, point, fact, argIndex, args[argIndex], want, env); ok {
			extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
			return objectLiteralCallParamObligationDiagnostic(fact.Call, callObligationName(result, site), argIndex, args[argIndex], mismatch, extra...), true
		}
		resolution := resolveCallParamObligationArgumentType(result, p.resolver, point, fact, argIndex, args[argIndex], env, producerContext(p), inherited)
		got := resolution.Type
		if containsTypeParamSyntax(got) {
			continue
		}
		readBoundary := boundaryCallArgumentReader(fact, argIndex, args[argIndex])
		untrustedTopLike := resolution.UntrustedTopLike
		if untrustedTopLike {
			if !boundaryProofTypeMismatch(result, point, got, want, readBoundary) {
				continue
			}
		} else if !callParamObligationTypeMismatch(result, point, got, want, readBoundary, args[argIndex]) {
			continue
		}
		if contextualParameterArgumentOwnedByCallSite(result, producerContext(p), args[argIndex]) {
			continue
		}
		hasUntrustedBoundary := boundaryValueHasUntrustedTopOrigin(result, point, readBoundary)
		proofBoundaryMessage := untrustedTopLike && hasUntrustedBoundary
		evidenceSubject := callArgumentBoundaryEvidenceSubject(result, args[argIndex], argIndex, hasUntrustedBoundary)
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(args[argIndex]), evidenceSubject, want, readBoundary)
		if len(extra) == 0 {
			extra = explicitTopLikeCastEvidence(ast.SpanOf(args[argIndex]), want, args[argIndex])
		}
		if len(extra) == 0 && topLikeType(got) {
			extra = callParamObligationMissingProofEvidence(ast.SpanOf(args[argIndex]), callArgumentSubject(argIndex, exprEvidenceNameOK(args[argIndex])), want)
		}
		if callArgumentCascadesFromInvalidLocalDeclaration(result, producerContext(p), point, args[argIndex], want, inherited) {
			continue
		}
		return callParamObligationDiagnostic(fact.Call, callObligationName(result, site), argIndex, obligation, got, want, args[argIndex], proofBoundaryMessage, extra...), true
	}
	return diagnostic.Diagnostic{}, false
}

func callArgumentBoundaryEvidenceSubject(result *body.Result, arg ast.Expr, argIndex int, proofBoundary bool) string {
	subject := callArgumentSubject(argIndex, exprEvidenceNameOK(arg))
	if !proofBoundary || result == nil {
		return subject
	}
	if cast, ok := arg.(*ast.CastExpr); ok && !topLikeCastTargetExpr(cast) {
		return subject
	}
	if result.Function() != nil {
		return unknownSourceName
	}
	p, ok := result.ExpressionPath(arg)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return subject
	}
	if kind, ok := result.SymbolKind(p.Symbol); ok && kind == symbol.Param {
		return unknownSourceName
	}
	return subject
}

func objectLiteralCallParamObligationDiagnostic(call *ast.FuncCallExpr, name string, index int, arg ast.Expr, mismatch objectLiteralTypeMismatch, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	subject := fmt.Sprintf("argument %d", index+1)
	if mismatch.suffix != "" {
		subject += mismatch.suffix
	}
	frameExpr := mismatch.expr
	if frameExpr == nil {
		frameExpr = arg
	}
	evidence := []diagnostic.Evidence{{
		Kind:    diagnostic.EvidenceAbstractFact,
		Trust:   diagnostic.TrustProven,
		Span:    ast.SpanOf(frameExpr),
		Message: callParameterTypeEvidence(name, index+1, mismatch.suffix, mismatch.want),
	}}
	evidence = append(evidence, extraEvidence...)
	evidence = append(evidence, mismatch.missingMemberEvidence()...)
	evidence = append(evidence, mismatch.unionArmEvidence...)
	message := fmt.Sprintf("%s is %s, not %s", subject, formatType(mismatch.got), formatType(mismatch.want))
	if mismatch.missingMethod.Name != "" {
		message = fmt.Sprintf("%s does not implement %s: missing method %q", subject, formatType(mismatch.want), mismatch.missingMethod.Name)
	}
	return argTypeDiagnosticEnvelopeWithSubject(call, frameExpr, index, mismatch.got, "", subject, message, evidence[0], evidence[1:]...)
}

func resolveCallParamObligationArgumentType(
	result *body.Result,
	resolver typeannotation.Resolver,
	point cfg.Point,
	fact semantics.CallFact,
	argIndex int,
	arg ast.Expr,
	env guardEnv,
	context producerContext,
	defs map[symbol.ID]*ast.FunctionExpr,
) diagnosticTypeResolution {
	var boundaryTopLike typ.Type
	if got, ok := concreteCastObligationType(result, resolver, point, env, arg); ok {
		return diagnosticTypeResolution{Type: got, Source: "concrete-cast-obligation", OK: true}
	}
	if direct := resolveDirectCallArgumentSourceType(result, resolver, point, env, fact, argIndex, arg, context, defs); direct.OK {
		if topLikeType(direct.Type) {
			boundaryTopLike = direct.Type
		} else {
			return diagnosticTypeResolution{
				Type:             direct.Type,
				Source:           "direct-call-argument-source",
				UntrustedTopLike: direct.UntrustedTopLike,
				OK:               true,
			}
		}
	}
	if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, arg); ok {
		return diagnosticTypeResolution{Type: got, Source: "untrusted-top-like-expression", UntrustedTopLike: true, OK: true}
	}
	if got, ok := callParamObligationSignatureArgumentType(result, point, argIndex); ok {
		return diagnosticTypeResolution{Type: got, Source: "signature-argument-type", OK: true}
	}
	if got, ok := directCallArgumentContractSourceType(result, context, fact, argIndex, defs); ok && !topLikeType(got) {
		return diagnosticTypeResolution{Type: got, Source: "call-result-contract-source", OK: true}
	}
	readBoundary := boundaryCallArgumentReader(fact, argIndex, arg)
	if boundary, ok := boundaryCallArgumentReaderType(result, resolver, point, readBoundary, arg); ok {
		untrusted := boundaryValueHasUntrustedTopOrigin(result, point, readBoundary)
		if topLikeType(boundary) {
			boundaryTopLike = boundary
		} else {
			return diagnosticTypeResolution{Type: boundary, Source: "boundary-call-argument-reader", UntrustedTopLike: untrusted, OK: true}
		}
	}
	if boundary, ok := boundaryExpressionValueType(result, point, arg); ok {
		untrusted := expressionValueHasUntrustedTopOrigin(result, point, arg)
		if projectionHasNil(boundary) {
			if projected, ok := projectedStructuralFlowSourceType(result, resolver, point, env, arg); ok && !projectionHasNil(projected) {
				return diagnosticTypeResolution{Type: projected, Source: "projected-structural-flow-source", UntrustedTopLike: untrusted, OK: true}
			}
		}
		if topLikeType(boundary) {
			boundaryTopLike = boundary
		} else {
			return diagnosticTypeResolution{Type: boundary, Source: "boundary-expression-value", UntrustedTopLike: untrusted, OK: true}
		}
	}
	if projected, ok := projectedStructuralFlowSourceType(result, resolver, point, env, arg); ok {
		return diagnosticTypeResolution{Type: projected, Source: "projected-structural-flow-source", OK: true}
	}
	if got, ok := guardedFlowExpressionType(result, resolver, point, env, arg); ok {
		return diagnosticTypeResolution{Type: got, Source: "guarded-flow-expression", OK: true}
	}
	if got, ok := boundaryCallArgumentSourceType(result, point, fact, argIndex); ok {
		if topLikeType(got) {
			if boundaryTopLike == nil {
				boundaryTopLike = got
			}
		} else {
			return diagnosticTypeResolution{Type: got, Source: "boundary-call-argument-source", OK: true}
		}
	}
	if got, ok := staticExpressionType(result, resolver, arg); ok {
		return diagnosticTypeResolution{Type: got, Source: "static-expression", OK: true}
	}
	if boundaryTopLike != nil {
		return diagnosticTypeResolution{Type: boundaryTopLike, Source: "boundary-expression-value", OK: true}
	}
	return diagnosticTypeResolution{Type: typ.Unknown, Source: "unknown", OK: true}
}

func callParamObligationSignatureArgumentType(result *body.Result, point cfg.Point, index int) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	site, ok := result.CallSite(point)
	if !ok {
		return nil, false
	}
	source, ok := site.ArgumentSourceAt(index)
	if !ok {
		return nil, false
	}
	got, ok := result.SignatureArgumentTypeAtBoundary(point, source)
	if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
}

func originObligationForParam(result *body.Result, obligations []callpayload.CallParamObligation, param int, want typ.Type) (callpayload.CallParamObligation, bool) {
	for _, candidate := range obligations {
		if candidate.ParamIndex != param || !candidate.Origin.HasOrigin {
			continue
		}
		candidateWant, ok := newDiagnosticQuery(result).ValueType(candidate.Value)
		if !ok || candidateWant == nil || !typ.TypeEquals(candidateWant, want) {
			continue
		}
		return candidate, true
	}
	return callpayload.CallParamObligation{}, false
}

func memberCallContractWouldReport(result *body.Result, point cfg.Point, fact semantics.CallFact, site factflow.CallSite, context producerContext, env guardEnv) bool {
	if !site.CalleeMemberAccess() {
		return false
	}
	_, ok := memberCall(context).call(result, point, fact, site, env)
	return ok
}

func directMemberCallContractWouldReport(result *body.Result, point cfg.Point, fact semantics.CallFact, site factflow.CallSite, context producerContext, env guardEnv) bool {
	if !site.CalleeMemberAccess() {
		return false
	}
	contract, ok := currentDirectFunctionContract(result, context, point, fact, site, "member call", nil)
	if !ok {
		return false
	}
	_, ok = directFunctionCallContractDiagnostic(context, result, point, fact, site, contract, nil, env)
	return ok
}

func boundaryExpressionValueType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	value, ok := newDiagnosticQuery(result).ExpressionValueAtBoundary(point, expr)
	if !ok {
		return nil, false
	}
	return newDiagnosticQuery(result).ValueTypeWithPresence(value)
}

func callParamObligationDiagnostic(
	call *ast.FuncCallExpr,
	name string,
	index int,
	obligation callpayload.CallParamObligation,
	got typ.Type,
	want typ.Type,
	arg ast.Expr,
	proofBoundaryMessage bool,
	extraEvidence ...diagnostic.Evidence,
) diagnostic.Diagnostic {
	argName := exprEvidenceName(arg)
	argSpan := spanWithEvidenceName(ast.SpanOf(arg), argName)
	subject := callArgumentSubject(index, exprEvidenceNameOK(arg))
	if proofBoundaryMessage {
		return argTypeDiagnosticEnvelope(call, arg, index, got,
			argumentBoundaryProofMessage(fmt.Sprintf("argument %d", index+1), arg, want),
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    argSpan,
				Message: callParamObligationEvidenceMessage(name, subject, obligation, want),
			},
			"",
			extraEvidence...)
	}
	return argTypeDiagnosticEnvelope(call, arg, index, got,
		argumentTypeMismatchMessage(fmt.Sprintf("argument %d", index+1), arg, got, want),
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    argSpan,
			Message: callParamObligationEvidenceMessage(name, subject, obligation, want),
		},
		"",
		extraEvidence...)
}

func callArgumentSubject(index int, argName string) string {
	base := fmt.Sprintf("argument %d", index+1)
	if argName == "" {
		return base
	}
	return fmt.Sprintf("%s (%s)", base, argName)
}

func callParamObligationEvidenceMessage(name string, subject string, obligation callpayload.CallParamObligation, want typ.Type) string {
	if !obligation.Origin.HasOrigin {
		return callParamObligationEvidence(name, subject, want)
	}
	provider := memberCallObligationProviderDisplay(obligation.Origin)
	return memberCallParamObligationEvidence(name, subject, provider, obligation.Origin.MemberParamIndex+1, want)
}

func memberCallObligationProviderDisplay(origin callpayload.CallParamObligationOrigin) string {
	var segs []segment.Segment
	if origin.ReceiverPath != "" {
		var ok bool
		segs, ok = pathaddr.RelativeStaticMemberSuffixSegments(origin.ReceiverPath)
		if !ok {
			return argumentMemberDisplay(origin.ReceiverParam+1, nil, origin.Member)
		}
	}
	segs = append(segs, origin.Member)
	return argumentMemberDisplay(origin.ReceiverParam+1, segs, origin.Member)
}

func argumentMemberDisplay(argument int, segs []segment.Segment, fallback segment.Segment) string {
	display := segment.FormatSegments(segs)
	if display == "" {
		display = memberSegmentDisplay(fallback)
		if display != "" && display[0] != '[' && display[0] != '.' {
			display = "." + display
		}
	}
	return fmt.Sprintf("argument %d%s", argument, display)
}

func callParamObligationMissingProofEvidence(span diagnostic.Span, subject string, want typ.Type) []diagnostic.Evidence {
	return []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			Span:    span,
			Message: missingBoundaryProofMessageForSubject(subject, want),
		},
	}
}

func callParamObligationTypeMismatch(result *body.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader, arg ast.Expr) bool {
	if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if !topLikeType(got) {
		return clearMismatch(result, got, want)
	}
	if read != nil {
		if value, ok := read(result, point); ok {
			if t, ok := newDiagnosticQuery(result).ValueType(value); ok && !topLikeType(t) && !boundaryTypeMismatch(result, point, t, want, nil) {
				return false
			}
		}
	}
	// A top-like got is an unresolved argument type. Inline table/function
	// literals are typed and validated by structural producers, so reporting them
	// here as unknown mismatches is a duplicate false positive. Attribute reads
	// are different: an unresolved `config.name` passed to a concrete parameter is
	// exactly the missing-proof boundary this producer exists to report.
	return obligationReportableArgument(arg)
}

// obligationReportableArgument reports whether a top-like (unresolved) argument
// expression should be reported against a concrete callee obligation. Inline
// table literals and member-function references carry their own structural
// validation, so they are excluded; plain value references remain reportable.
func obligationReportableArgument(arg ast.Expr) bool {
	switch a := arg.(type) {
	case *ast.TableExpr, *ast.FunctionExpr:
		return false
	case *ast.AttrGetExpr:
		return true
	case *ast.CastExpr:
		return obligationReportableArgument(a.Expr)
	default:
		return true
	}
}

func callObligationName(result *body.Result, site factflow.CallSite) string {
	if receiver, member, ok := site.CalleeMemberAccessPath(); ok {
		memberName := memberSegmentDisplay(member)
		if memberName == "" {
			return "call target"
		}
		name := ""
		if result != nil && receiver.Symbol != 0 {
			name = result.SymbolName(receiver.Symbol)
		}
		if name == "" {
			name = "receiver"
		}
		return memberPathName(name, memberName)
	}
	if result != nil {
		if name := result.SymbolName(site.CalleeSymbol()); name != "" {
			return name
		}
	}
	return "call target"
}

func explicitContractParamCoversObligation(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	index int,
	want typ.Type,
	resolver typeannotation.Resolver,
	inherited map[symbol.ID]*ast.FunctionExpr,
) bool {
	if result == nil || fact.Call == nil || index < 0 || want == nil {
		return false
	}
	if site.CalleeMemberAccess() {
		return false
	}
	argIndex := explicitArgumentIndexForParam(fact, index)
	if argIndex >= 0 && fact.Call != nil && argIndex < len(fact.Call.Args) {
		readBoundary := boundaryCallArgumentReader(fact, argIndex, fact.Call.Args[argIndex])
		if boundaryValueHasUntrustedTopOrigin(result, point, readBoundary) {
			return false
		}
	}
	if site.CalleeSymbol() == 0 {
		return false
	}
	if def := inherited[site.CalleeSymbol()]; def != nil {
		contract, ok := lowerDirectFunctionContractInResultScope(result, def, resolver)
		return ok && directContractParamCoversObligation(contract, index, want)
	}
	if fn, ok := result.CallSignatureType(site); ok {
		return directContractParamCoversObligation(lowerDirectFunctionType(fn), index, want)
	}
	baseExpr, ok := result.SymbolTypeAnnotation(site.CalleeSymbol())
	if !ok {
		return false
	}
	baseType, ok := lowerType(baseExpr, resolver)
	if !ok || typ.IsAny(baseType) || typ.IsUnknown(baseType) {
		return false
	}
	callable, ok := typecall.Callable(baseType)
	return ok && directContractParamCoversObligation(lowerDirectFunctionType(callable), index, want)
}

func explicitArgumentIndexForParam(fact semantics.CallFact, paramIndex int) int {
	if fact.Receiver != nil && fact.Method != "" {
		return paramIndex - 1
	}
	return paramIndex
}

func directContractParamCoversObligation(contract directFunctionContract, index int, want typ.Type) bool {
	if index < len(contract.params) {
		param := contract.params[index]
		return directParamCoversObligation(param, want)
	}
	if contract.hasVararg {
		return directParamCoversObligation(contract.variadic, want)
	}
	return false
}

func directParamCoversObligation(param directCallParam, want typ.Type) bool {
	return param.explicit &&
		param.typ != nil &&
		!typ.IsAny(param.typ) &&
		!typ.IsUnknown(param.typ) &&
		!refinement.ContainsFreeTypeParam(param.typ) &&
		(!clearMismatch(nil, param.typ, want) || directParamCoversNonnilProjection(param.typ, want))
}

func directParamCoversNonnilProjection(paramType, want typ.Type) bool {
	if paramType == nil || want == nil || !projectionHasNil(paramType) {
		return false
	}
	withoutNil := projectionWithoutNil(paramType)
	if withoutNil == nil || typ.IsNever(withoutNil) {
		return false
	}
	return !clearMismatch(nil, withoutNil, want)
}
