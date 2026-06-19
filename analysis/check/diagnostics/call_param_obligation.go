package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
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
	envs := cachedGuardEnvironments(result)
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
		outcome, ok := result.CallOutcomeAt(point)
		if !ok || len(outcome.ParamObligations) == 0 {
			continue
		}
		if d, ok := producer.call(result, point, fact, outcome.ParamObligations, inherited, envs[point]); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p callParamObligations) call(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	obligations []callpayload.CallParamObligation,
	inherited map[symbol.ID]*ast.FunctionExpr,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	args := fact.Call.Args
	if memberCallContractWouldReport(result, point, fact, producerContext(p), env) ||
		directMemberCallContractWouldReport(result, point, fact, producerContext(p), env) {
		return diagnostic.Diagnostic{}, false
	}
	// Colon method calls pass the receiver as the implicit first parameter, so
	// obligation parameter indices (function-parameter space, self at 0) are
	// shifted by one relative to the explicit argument list.
	receiverOffset := 0
	if fact.Receiver != nil && fact.Method != "" {
		receiverOffset = 1
	}
	seen := make(map[int]struct{}, len(obligations))
	for _, obligation := range obligations {
		i := obligation.ParamIndex
		argIndex := i - receiverOffset
		if argIndex < 0 || argIndex >= len(args) {
			continue
		}
		if _, ok := seen[argIndex]; ok {
			continue
		}
		want, ok := readmodel.New(result).ValueType(obligation.Value)
		if !ok || want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		if !obligation.Origin.HasOrigin {
			if richer, ok := originObligationForParam(result, obligations, i, want); ok {
				obligation = richer
			}
		}
		if explicitContractParamCoversObligation(result, point, fact, i, want, p.resolver, inherited) {
			continue
		}
		seen[argIndex] = struct{}{}
		if mismatch, ok := objectLiteralMemberMismatch(result, point, args[argIndex], want, env); ok {
			extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
			return callParamObligationDiagnostic(fact.Call, callObligationName(result, fact), argIndex, obligation, mismatch.got, mismatch.want, mismatch.expr, extra...), true
		}
		got, ok := untrustedTopLikeExpressionTypeAt(result, p.resolver, point, args[argIndex])
		untrustedTopLike := ok
		if !ok {
			got, ok = projectedStructuralFlowSourceType(result, p.resolver, point, guardEnv{}, args[argIndex])
		}
		if !ok {
			got, ok = boundaryCallArgumentSourceType(result, point, fact, argIndex)
		}
		if !ok {
			got, ok = boundaryExprType(result, p.resolver, args[argIndex])
		}
		if !ok {
			got, ok = boundaryExpressionType(result, point, args[argIndex])
		}
		if !ok {
			got = typ.Unknown
		}
		readBoundary := boundaryCallArgumentReader(fact, argIndex, args[argIndex])
		if untrustedTopLike {
			if !boundaryProofTypeMismatch(result, point, got, want, readBoundary) {
				continue
			}
		} else if !callParamObligationTypeMismatch(result, point, got, want, readBoundary, args[argIndex]) {
			continue
		}
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(args[argIndex]), callArgumentSubject(argIndex, exprEvidenceNameOK(args[argIndex])), want, readBoundary)
		if len(extra) == 0 {
			extra = explicitTopLikeCastEvidence(ast.SpanOf(args[argIndex]), want, args[argIndex])
		}
		if len(extra) == 0 && topLikeType(got) {
			extra = callParamObligationMissingProofEvidence(ast.SpanOf(args[argIndex]), callArgumentSubject(argIndex, exprEvidenceNameOK(args[argIndex])), want)
		}
		return callParamObligationDiagnostic(fact.Call, callObligationName(result, fact), argIndex, obligation, got, want, args[argIndex], extra...), true
	}
	return diagnostic.Diagnostic{}, false
}

func originObligationForParam(result *body.Result, obligations []callpayload.CallParamObligation, param int, want typ.Type) (callpayload.CallParamObligation, bool) {
	for _, candidate := range obligations {
		if candidate.ParamIndex != param || !candidate.Origin.HasOrigin {
			continue
		}
		candidateWant, ok := readmodel.New(result).ValueType(candidate.Value)
		if !ok || candidateWant == nil || !typ.TypeEquals(candidateWant, want) {
			continue
		}
		return candidate, true
	}
	return callpayload.CallParamObligation{}, false
}

func memberCallContractWouldReport(result *body.Result, point cfg.Point, fact semantics.CallFact, context producerContext, env guardEnv) bool {
	if _, _, _, member := callMemberAccess(fact); !member {
		return false
	}
	_, ok := memberCall(context).call(result, point, fact, env)
	return ok
}

func directMemberCallContractWouldReport(result *body.Result, point cfg.Point, fact semantics.CallFact, context producerContext, env guardEnv) bool {
	if _, _, _, member := callMemberAccess(fact); !member {
		return false
	}
	contract, ok := currentDirectFunctionContract(result, context, point, fact, "member call", nil)
	if !ok {
		return false
	}
	_, ok = directCallContract(context).directFunctionCall(result, point, fact, contract, nil, env)
	return ok
}

func boundaryExpressionType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	value, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return nil, false
	}
	return readmodel.New(result).ValueType(value)
}

func callParamObligationDiagnostic(
	call *ast.FuncCallExpr,
	name string,
	index int,
	obligation callpayload.CallParamObligation,
	got typ.Type,
	want typ.Type,
	arg ast.Expr,
	extraEvidence ...diagnostic.Evidence,
) diagnostic.Diagnostic {
	argName := exprEvidenceName(arg)
	argSpan := spanWithEvidenceName(ast.SpanOf(arg), argName)
	subject := callArgumentSubject(index, exprEvidenceNameOK(arg))
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
	segs, ok := segment.ParseFormattedSegments(string(origin.ReceiverPath))
	if !ok {
		return argumentMemberDisplay(origin.ReceiverParam+1, nil, origin.Member)
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
			if t, ok := readmodel.New(result).ValueType(value); ok && !topLikeType(t) && !boundaryTypeMismatch(result, point, t, want, nil) {
				return false
			}
		}
	}
	// A top-like got is an unresolved argument type. Inline table literals and
	// member-function references are typed and validated by structural producers,
	// so reporting them here as unknown mismatches is a duplicate false positive;
	// every other argument form remains reportable against the concrete
	// obligation.
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
		return a.KeySyntax == ast.AttrKeyIndex
	case *ast.CastExpr:
		return obligationReportableArgument(a.Expr)
	default:
		return true
	}
}

func callObligationName(result *body.Result, fact semantics.CallFact) string {
	if result != nil && fact.HasCalleeSymbol {
		if name := result.SymbolName(fact.CalleeSymbol); name != "" {
			return name
		}
	}
	if receiver, member, _, ok := callMemberAccess(fact); ok && member != "" {
		name := ""
		if result != nil && receiver.Symbol != 0 {
			name = result.SymbolName(receiver.Symbol)
		}
		if name == "" {
			name = "receiver"
		}
		return memberPathName(name, member)
	}
	return "call target"
}

func explicitContractParamCoversObligation(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	index int,
	want typ.Type,
	resolver typeannotation.Resolver,
	inherited map[symbol.ID]*ast.FunctionExpr,
) bool {
	if result == nil || fact.Call == nil || index < 0 || want == nil {
		return false
	}
	if _, _, _, member := callMemberAccess(fact); member {
		return false
	}
	site, ok := result.CallSite(point)
	if !ok || site.CalleeSymbol() == 0 {
		return false
	}
	if def := inherited[site.CalleeSymbol()]; def != nil {
		contract, ok := lowerDirectFunctionContractInResultScope(result, def, resolver)
		return ok && directContractParamCoversObligation(contract, index, want)
	}
	if sig, ok := result.CallSignature(site); ok && sig.Type != nil {
		return directContractParamCoversObligation(lowerDirectFunctionType(sig.Type), index, want)
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
