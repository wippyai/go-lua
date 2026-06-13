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
		if d, ok := producer.call(result, point, fact, outcome.ParamObligations, inherited); ok {
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
) (diagnostic.Diagnostic, bool) {
	args := fact.Call.Args
	seen := make(map[int]struct{}, len(obligations))
	for _, obligation := range obligations {
		i := obligation.ParamIndex
		if i < 0 || i >= len(args) {
			continue
		}
		if _, ok := seen[i]; ok {
			continue
		}
		seen[i] = struct{}{}
		if explicitContractParam(result, point, fact, i, p.resolver, inherited) {
			continue
		}
		want, ok := readmodelType(result, obligation.Value)
		if !ok || want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		got, ok := projectedFlowSourceType(result, p.resolver, point, literalEnv{}, args[i])
		if !ok {
			got, ok = boundaryCallArgumentSourceType(result, point, fact, i)
		}
		if !ok {
			got, ok = boundaryExprType(result, p.resolver, args[i])
		}
		if !ok {
			got, ok = boundaryExpressionType(result, point, args[i])
		}
		if !ok {
			got = typ.Unknown
		}
		if !callParamObligationTypeMismatch(result, point, got, want, boundaryCallArgumentReader(fact, i, args[i])) {
			continue
		}
		return callParamObligationDiagnostic(point, fact.Call, callObligationName(result, fact), i, got, want, args[i]), true
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
	point cfg.Point,
	call *ast.FuncCallExpr,
	name string,
	index int,
	got typ.Type,
	want typ.Type,
	arg ast.Expr,
) diagnostic.Diagnostic {
	d := argTypeDiagnostic(point, call, name, index, got, want, arg, ast.SpanOf(call))
	d.Message = fmt.Sprintf("argument %d expected %s, got %s", index+1, formatType(want), formatType(got))
	return d
}

func callParamObligationTypeMismatch(result *body.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader) bool {
	if want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if !topLikeType(got) {
		return clearMismatch(got, want)
	}
	if read != nil {
		if value, ok := read(result, point); ok {
			if t, ok := readmodel.New(result).ValueType(value); ok && !topLikeType(t) && !boundaryTypeMismatch(result, point, t, want, nil) {
				return false
			}
		}
	}
	return true
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
