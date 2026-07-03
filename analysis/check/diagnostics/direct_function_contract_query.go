package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func currentDirectFunctionContract(
	result *body.Result,
	context producerContext,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	name string,
	def *ast.FunctionExpr,
) (directFunctionContract, bool) {
	if fact.Call == nil || fact.Call.Func == nil {
		return directFunctionContract{}, false
	}
	flow := context.flow
	if flow == nil {
		flow = newDiagnosticFlowCache(result)
	}
	calleeSymbol := site.CalleeSymbol()
	symbolReassigned := calleeSymbol != 0 &&
		flow.directFunctionReassignedAfterDefinition(point, calleeSymbol)
	calleePath := site.CalleePathRef()
	if !calleePath.IsEmpty() {
		if fn, defPoint, ok := dominatingFunctionDefinitionForPathWithPoint(result, point, calleePath); ok &&
			!memberPathReassignedAfterDefinition(result, context.flow, defPoint, point, calleePath) {
			contract, ok := memberCall(context).memberFunctionDefinitionContract(result, fn)
			if ok {
				if lossyImplicitSelfMemberFallback(result, fact, site, contract) {
					return directFunctionContract{}, false
				}
				contract.name = name
				contract.declSpan = ast.SpanOf(fn)
				return contract, true
			}
		}
	}
	if context.callContextResult || symbolReassigned || def == nil {
		if contract, ok := currentFunctionValueContract(result, context, point, fact, site, name); ok {
			return contract, true
		}
	}
	if def != nil && !symbolReassigned {
		contract, ok := lowerDirectFunctionContractInResultScope(result, def, context.resolver)
		if ok {
			if lossyImplicitSelfMemberFallback(result, fact, site, contract) {
				return directFunctionContract{}, false
			}
			contract.name = name
			contract.declSpan = ast.SpanOf(def)
			return contract, true
		}
	}
	if calleeSymbol != 0 && !symbolReassigned {
		if fn, ok := result.FunctionBySymbol(calleeSymbol); ok && fn != nil {
			contract, ok := lowerDirectFunctionContractInResultScope(result, fn, context.resolver)
			if ok {
				if lossyImplicitSelfMemberFallback(result, fact, site, contract) {
					return directFunctionContract{}, false
				}
				contract.name = name
				contract.declSpan = ast.SpanOf(fn)
				return contract, true
			}
		}
	}
	calleeType, ok := directCallCalleeType(result, context, point, fact.Call.Func)
	if !ok || typ.IsAny(calleeType) || typ.IsUnknown(calleeType) {
		return directFunctionContract{}, false
	}
	callable, ok := typecall.Callable(calleeType)
	if !ok || callable == nil {
		return directFunctionContract{}, false
	}
	contract := lowerDirectFunctionType(callable)
	if lossyImplicitSelfMemberFallback(result, fact, site, contract) {
		return directFunctionContract{}, false
	}
	contract.name = name
	contract.declSpan = ast.SpanOf(fact.Call)
	return contract, true
}

func currentFunctionValueContract(
	result *body.Result,
	context producerContext,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	name string,
) (directFunctionContract, bool) {
	if result == nil || fact.Call == nil || fact.Call.Func == nil {
		return directFunctionContract{}, false
	}
	fn, ok := result.FunctionValueTypeAtBoundary(point, fact.Call.Func)
	if !ok || fn == nil {
		return directFunctionContract{}, false
	}
	contract := lowerDirectFunctionType(fn)
	if lossyImplicitSelfMemberFallback(result, fact, site, contract) {
		return directFunctionContract{}, false
	}
	contract.name = name
	contract.declSpan = ast.SpanOf(fact.Call.Func)
	return contract, true
}

func lossyImplicitSelfMemberFallback(result *body.Result, fact semantics.CallFact, site factflow.CallSite, contract directFunctionContract) bool {
	if result == nil || fact.Call == nil || len(fact.Call.Args) == 0 || len(contract.params) != 0 {
		return false
	}
	if _, _, ok := site.CalleeMemberAccessPath(); !ok {
		return false
	}
	return implicitSelfArgumentInResultTree(result, fact.Call.Args[0])
}
