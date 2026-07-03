package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func bindDirectCallReceiver(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	contract directFunctionContract,
	resolver typeannotation.Resolver,
	env guardEnv,
) directFunctionContract {
	if fact.Call == nil || fact.Call.Method == "" || fact.Call.Receiver == nil {
		return contract
	}
	var receiverType typ.Type
	receiverType, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(fact.Call.Receiver)
	if !ok {
		receiverType, ok = newStructuralFlowExpressionTyper(result, resolver, point, env).typeOf(fact.Call.Receiver)
	}
	if receiverType != nil && contract.source != nil && typecall.CallableConsumesReceiver(contract.source, receiverType) {
		return colonMemberCallContract(receiverType, contract)
	}
	if directCallContractHasUnboundReceiverSlot(contract) {
		if receiverType == nil {
			return memberContractWithoutReceiver(contract)
		}
		return colonMemberCallContract(receiverType, contract)
	}
	if declaredType, declaredOK := directCallDeclaredReceiverType(result, resolver, site, fact); declaredOK &&
		directCallContractHasInterfaceReceiverSlot(contract) &&
		colonMemberCallConsumesReceiver(contract, declaredType) {
		return colonMemberCallContract(declaredType, contract)
	}
	return contract
}

func directCallDeclaredReceiverType(result *body.Result, resolver typeannotation.Resolver, site factflow.CallSite, fact semantics.CallFact) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	receiver, ok := site.ReceiverPath()
	if (!ok || receiver.Symbol == 0) && fact.Call != nil && fact.Call.Receiver != nil {
		p, ok := result.ExpressionPath(fact.Call.Receiver)
		if !ok {
			return nil, false
		}
		receiver = p
	}
	if receiver.Symbol == 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(receiver.Symbol)
	if !ok {
		return nil, false
	}
	return lowerType(annotation, resolver)
}

func directCallContractHasUnboundReceiverSlot(contract directFunctionContract) bool {
	if len(contract.params) == 0 {
		return false
	}
	first := contract.params[0]
	if first.implicitSelf || typecall.ParamConsumesReceiver(first.display, first.typ, nil) {
		return true
	}
	if contract.source == nil || len(contract.source.Params) == 0 {
		return false
	}
	sourceFirst := contract.source.Params[0]
	return typecall.ParamConsumesReceiver(sourceFirst.Name, sourceFirst.Type, nil) && typ.TypeEquals(first.typ, sourceFirst.Type)
}

func directCallContractHasInterfaceReceiverSlot(contract directFunctionContract) bool {
	if len(contract.params) == 0 {
		return false
	}
	_, ok := diagnosticInterfaceBody(unwrap.Annotated(contract.params[0].typ))
	return ok
}

func contextIndependentImplicitSelfArgument(result *body.Result, arg ast.Expr) bool {
	if result == nil || result.IsCallContextResult() || arg == nil {
		return false
	}
	return implicitSelfArgumentInResult(result, arg)
}

func implicitSelfArgumentInResultTree(result *body.Result, arg ast.Expr) bool {
	if implicitSelfArgumentInResult(result, arg) {
		return true
	}
	for _, child := range result.FunctionResults() {
		if implicitSelfArgumentInResultTree(child, arg) {
			return true
		}
	}
	return false
}

func implicitSelfArgumentInResult(result *body.Result, arg ast.Expr) bool {
	if result == nil || arg == nil {
		return false
	}
	fn := result.Function()
	if fn == nil {
		return false
	}
	origin, ok := result.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginMethod {
		return false
	}
	argPath, ok := result.ExpressionPath(arg)
	if !ok || argPath.Symbol == 0 || len(argPath.Segments) != 0 {
		return false
	}
	for _, slot := range result.FunctionParamSlots(fn) {
		if slot.ImplicitSelf && slot.Symbol == argPath.Symbol {
			return true
		}
	}
	return false
}
