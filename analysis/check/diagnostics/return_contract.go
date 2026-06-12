package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ReturnContract reports explicit return annotation mismatches that are already
// proven by the solved return facts.
type ReturnContract Config

func (p ReturnContract) Produce(result *check.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	fn := result.Function()
	if fn == nil {
		return nil
	}
	returns, ok := lowerReturnTypes(fn, p.Resolver)
	if !ok || len(returns) == 0 {
		return nil
	}
	var out []diagnostic.Diagnostic
	for _, point := range result.ReturnPoints() {
		fact, ok := result.ReturnFact(point)
		if !ok || len(fact.Exprs) == 0 {
			continue
		}
		for i, expr := range fact.Exprs {
			source := returnSourceAt(fact, i)
			got, ok := returnValueType(result, p.Resolver, expr, source)
			if !ok {
				continue
			}
			want, ok := returnTypeAt(returns, i)
			if !ok || refinement.ContainsFreeTypeParam(want) {
				continue
			}
			if !boundaryTypeMismatch(result, point, got, want, boundaryValueFromASTSource(source)) {
				continue
			}
			annotation := typeExprAt(fn.ReturnTypes, i)
			if annotation == nil {
				continue
			}
			out = append(out, returnContractDiagnostic(expr, annotation, got, want, i))
		}
	}
	return out
}

func returnValueType(result *check.Result, resolver typeannotation.Resolver, expr ast.Expr, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if got, ok := valueexpr.LiteralType(expr); ok {
		return got, true
	}
	if got, ok := projectedOptionalIndexType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := explicitTopLikeExpressionType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallFactSourceType(result, resolver, source); ok {
		return got, true
	}
	return nil, false
}

func returnSourceAt(fact semantics.ReturnFact, index int) sourceprovenance.ASTSource {
	if index >= 0 && index < len(fact.Sources) {
		return fact.Sources[index]
	}
	return sourceprovenance.NewUnknownSource(factflow.NoValueSourceIndex)
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

func returnContractDiagnostic(expr ast.Expr, annotation ast.TypeExpr, got, want typ.Type, index int) diagnostic.Diagnostic {
	exprSpan := ast.SpanOf(expr)
	typeSpan := ast.SpanOf(annotation)
	label := "returned value"
	if index >= 0 {
		label = fmt.Sprintf("returned value %d", index+1)
	}
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      exprSpan.StartLine,
			Column:    exprSpan.StartCol,
			EndLine:   exprSpan.EndLine,
			EndColumn: exprSpan.EndCol,
		},
		Span:     exprSpan,
		Code:     CodeReturnContractType,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s is %s, not %s", label, formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    exprSpan,
				Message: fmt.Sprintf("%s is %s", label, formatType(got)),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    typeSpan,
				Message: fmt.Sprintf("declared return type is %s", formatType(want)),
			},
		),
		Labels: []diagnostic.Label{
			{Span: exprSpan, Message: "returned value"},
			{Span: typeSpan, Message: "declared return type"},
		},
	}
}

// DirectCallResultAssignment reports mismatches between direct-call return
// contracts and annotated local targets that receive those call results.
type DirectCallResultAssignment Config

func (p DirectCallResultAssignment) Produce(result *check.Result) []diagnostic.Diagnostic {
	return produceDirectCallResultAssignment(result, Config(p), nil)
}

func produceDirectCallResultAssignment(result *check.Result, config Config, inherited map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	defs := directCallDefinitions(result, inherited)
	producer := DirectCallResultAssignment(config)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok || site.CalleeSymbol() == 0 {
			continue
		}
		if _, _, _, ok := callMemberAccess(fact); ok {
			if _, hasSignature := result.CallSignature(site); !hasSignature {
				continue
			}
		}
		contract, name, ok := directCallResultContract(result, point, fact, site, defs[site.CalleeSymbol()], producer.Resolver)
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
			want, ok := lowerType(wantExpr, producer.Resolver)
			if !ok || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
				continue
			}
			resultIndex := target.ResultIndex
			got, ok := contract.returnType(resultIndex)
			if !ok {
				got, ok = contract.declaredReturnType(resultIndex)
			}
			if !ok || refinement.ContainsFreeTypeParam(got) {
				continue
			}
			if !boundaryTypeMismatch(result, point, got, want, boundaryCallResultReader(point, resultIndex)) {
				continue
			}
			out = append(out, directCallResultAssignmentDiagnostic(point, fact.Call, name, resultIndex, got, want, wantExpr))
		}
	}
	return out
}

func boundaryCallResultReader(callPoint cfg.Point, resultIndex int) boundaryValueReader {
	return func(result *check.Result, point cfg.Point) (product.Value, bool) {
		if resultIndex < 0 {
			return product.Value{}, false
		}
		source, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, resultIndex, callPoint, factflow.ValueSourceShape{})
		if !ok {
			return product.Value{}, false
		}
		return result.SourceValueAtBoundary(point, source)
	}
}

func directCallResultContract(result *check.Result, point cfg.Point, fact semantics.CallFact, site factflow.CallSite, def *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, string, bool) {
	name := result.SymbolName(site.CalleeSymbol())
	if name == "" {
		name = "call target"
	}
	if def != nil {
		contract, ok := lowerDirectFunctionContract(def, resolver)
		if !ok {
			return directFunctionContract{}, "", false
		}
		contract.name = name
		contract.declSpan = ast.SpanOf(def)
		if !directCallArgsCompatible(result, point, fact, contract, resolver) {
			return directFunctionContract{}, "", false
		}
		return contract, name, true
	}
	if sig, ok := result.CallSignature(site); ok && sig.Type != nil {
		contract := lowerDirectFunctionType(sig.Type)
		contract.name = name
		contract.declSpan = ast.SpanOf(fact.Call)
		if !directCallArgsCompatible(result, point, fact, contract, resolver) {
			return directFunctionContract{}, "", false
		}
		return contract, name, true
	}
	baseExpr, ok := result.SymbolTypeAnnotation(site.CalleeSymbol())
	if !ok {
		return directFunctionContract{}, "", false
	}
	baseType, ok := lowerType(baseExpr, resolver)
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
	if !directCallArgsCompatible(result, point, fact, contract, resolver) {
		return directFunctionContract{}, "", false
	}
	return contract, name, true
}

func directCallArgsCompatible(result *check.Result, point cfg.Point, fact semantics.CallFact, contract directFunctionContract, resolver typeannotation.Resolver) bool {
	if fact.Call == nil {
		return false
	}
	args := fact.Call.Args
	required := contract.requiredArity()
	if len(args) < required {
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
		got, ok := boundaryExprType(result, resolver, arg)
		if !ok {
			got, ok = boundaryCallArgumentSourceType(result, point, fact, i)
		}
		if !ok || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		if !boundaryTypeMismatch(result, point, got, want, boundaryCallArgumentReader(fact, i, arg)) {
			continue
		}
		return false
	}
	return true
}

func directCallResultAssignmentDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, index int, got, want typ.Type, annotation ast.TypeExpr) diagnostic.Diagnostic {
	callSpan := ast.SpanOf(call)
	typeSpan := ast.SpanOf(annotation)
	label := "call result"
	if index >= 0 {
		label = fmt.Sprintf("call result %d", index+1)
	}
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      callSpan.StartLine,
			Column:    callSpan.StartCol,
			EndLine:   callSpan.EndLine,
			EndColumn: callSpan.EndCol,
		},
		Span:     callSpan,
		Code:     CodeDirectCallResultAssignment,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s is %s, not %s", label, formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    callSpan,
				Message: fmt.Sprintf("call at CFG point %d returns %s", point, formatType(got)),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    typeSpan,
				Message: fmt.Sprintf("%s is annotated %s", name, formatType(want)),
			},
		),
		Labels: []diagnostic.Label{
			{Span: callSpan, Message: "call result"},
			{Span: typeSpan, Message: "declared type"},
		},
	}
}
