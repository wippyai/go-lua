package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
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
	envs := guardEnvironments(result)
	producer := callParamObligations(context)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
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
	obligations []factapply.CallParamObligation,
	inherited map[symbol.ID]*ast.FunctionExpr,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	args := fact.Call.Args
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
		seen[argIndex] = struct{}{}
		if explicitContractParam(result, point, fact, i, p.resolver, inherited) {
			continue
		}
		want, ok := readmodelType(result, obligation.Value)
		if !ok || want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		if mismatch, ok := objectLiteralMemberMismatch(result, point, args[argIndex], want, env); ok {
			extra := boundaryDiagnosticEvidence(result, point, ast.SpanOf(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
			return callParamObligationDiagnostic(fact.Call, callObligationName(result, fact), argIndex, mismatch.got, mismatch.want, mismatch.expr, extra...), true
		}
		got, ok := untrustedTopLikeExpressionTypeAt(result, p.resolver, point, args[argIndex])
		untrustedTopLike := ok
		if !ok {
			got, ok = projectedFlowSourceType(result, p.resolver, point, guardEnv{}, args[argIndex])
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
		extra := boundaryDiagnosticEvidence(result, point, ast.SpanOf(args[argIndex]), want, readBoundary)
		if len(extra) == 0 {
			extra = explicitTopLikeCastEvidence(ast.SpanOf(args[argIndex]), want, args[argIndex])
		}
		return callParamObligationDiagnostic(fact.Call, callObligationName(result, fact), argIndex, got, want, args[argIndex], extra...), true
	}
	return diagnostic.Diagnostic{}, false
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
	got typ.Type,
	want typ.Type,
	arg ast.Expr,
	extraEvidence ...diagnostic.Evidence,
) diagnostic.Diagnostic {
	d := argTypeDiagnostic(call, name, index, got, want, arg, ast.SpanOf(call), extraEvidence...)
	d.Message = fmt.Sprintf("argument %d expected %s, got %s", index+1, formatType(want), formatType(got))
	return d
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
	// A top-like got is an unresolved argument type. An inline table literal or a
	// member-function reference is typed and validated by its own structural
	// producer, so reporting it here as an unknown mismatch is a duplicate false
	// positive; every other argument form remains reportable against the concrete
	// obligation.
	return obligationReportableArgument(arg)
}

// obligationReportableArgument reports whether a top-like (unresolved) argument
// expression should be reported against a concrete summary obligation. Inline
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

func readmodelType(result *body.Result, value product.Value) (typ.Type, bool) {
	return readmodel.New(result).ValueType(value)
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
		return name + "." + member
	}
	return "call target"
}

func explicitContractParam(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	index int,
	resolver typeannotation.Resolver,
	inherited map[symbol.ID]*ast.FunctionExpr,
) bool {
	if result == nil || fact.Call == nil || index < 0 {
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
		contract, ok := lowerDirectFunctionContractInScope(def, resolver)
		return ok && directContractParamExplicit(contract, index)
	}
	if sig, ok := result.CallSignature(site); ok && sig.Type != nil {
		return directContractParamExplicit(lowerDirectFunctionType(sig.Type), index)
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
	return ok && directContractParamExplicit(lowerDirectFunctionType(callable), index)
}

func directContractParamExplicit(contract directFunctionContract, index int) bool {
	if index < len(contract.params) {
		param := contract.params[index]
		return param.explicit && param.typ != nil && !typ.IsAny(param.typ) && !typ.IsUnknown(param.typ)
	}
	if contract.hasVararg {
		param := contract.variadic
		return param.explicit && param.typ != nil && !typ.IsAny(param.typ) && !typ.IsUnknown(param.typ)
	}
	return false
}
