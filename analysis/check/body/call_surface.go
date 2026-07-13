package body

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// sealPreparedCallSurface independently enumerates lowering-owned calls and
// classifies their stable lexical targets. Facts are consulted only to bind an
// enumerated call to its semantic site; a missing fact remains an explicit
// rejected site rather than disappearing from the census.
func sealPreparedCallSurface(bindings *bind.Result, lowered *wir.Body, facts factflow.Facts, signatureCalls map[cfg.Point]operationplan.SignatureCallOperation, owner lexicalidentity.StableLexicalBodyID, namespace lexicalidentity.UnitNamespace, pointCount int) operationplan.CallSurface {
	if bindings == nil || lowered == nil || owner == (lexicalidentity.StableLexicalBodyID{}) || namespace == (lexicalidentity.UnitNamespace{}) {
		return operationplan.CallSurface{}
	}
	extracted := make([]cfg.Point, 0)
	sites := make([]operationplan.CallSurfaceSite, 0)
	for index := 0; index < lowered.Len(); index++ {
		instruction := lowered.Instr(index)
		if instruction.Op != wir.OpCall {
			continue
		}
		extracted = append(extracted, instruction.Point)
		target := operationplan.RejectedCallSurfaceTarget()
		if lexical, exact := exactPreparedLexicalCallTarget(bindings, namespace, lowered, instruction, facts); exact {
			target, _ = operationplan.NewLexicalCallSurfaceTarget(lexical)
		} else if external, exact := signatureCalls[instruction.Point]; exact && exactPreparedExternalCallShape(lowered, instruction, facts) {
			if sealed, ok := operationplan.NewExternalCallSurfaceTarget(external); ok {
				target = sealed
			}
		}
		sites = append(sites, operationplan.CallSurfaceSite{Point: instruction.Point, Target: target})
	}
	surface, err := operationplan.SealCallSurface(owner, pointCount, extracted, sites)
	if err != nil {
		return operationplan.CallSurface{}
	}
	return surface
}

func exactPreparedExternalCallShape(lowered *wir.Body, instruction wir.Instruction, facts factflow.Facts) bool {
	if lowered == nil || instruction.Call.Callee.Kind != wir.OperandPath ||
		instruction.Call.Receiver.Kind != wir.OperandNone || instruction.Call.Method != 0 {
		return false
	}
	calleePath := lowered.Path(wir.PathRef(instruction.Call.Callee.Ref))
	if calleePath.IsEmpty() || calleePath.Symbol == 0 {
		return false
	}
	site, ok := facts.CallSiteView(instruction.Point)
	if !ok || site.CalleeSymbol() != calleePath.Symbol || !site.CalleePathEqual(calleePath) ||
		site.CalleeMemberAccess() != (len(calleePath.Segments) != 0) || site.MethodName() != "" {
		return false
	}
	if _, method := site.ReceiverPath(); method {
		return false
	}
	if _, method := site.ReceiverSource(); method {
		return false
	}
	if _, method := site.MethodPath(); method {
		return false
	}
	return true
}

func exactPreparedLexicalCallTarget(bindings *bind.Result, namespace lexicalidentity.UnitNamespace, lowered *wir.Body, instruction wir.Instruction, facts factflow.Facts) (lexicalidentity.StableLexicalBodyID, bool) {
	if instruction.Call.Callee.Kind != wir.OperandPath || instruction.Call.Receiver.Kind != wir.OperandNone || instruction.Call.Method != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	path := lowered.Path(wir.PathRef(instruction.Call.Callee.Ref))
	if path.Symbol == 0 || len(path.Segments) != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	site, ok := facts.CallSiteView(instruction.Point)
	if !ok || site.CalleeSymbol() != path.Symbol || site.CalleeMemberAccess() || site.MethodName() != "" || site.TypeArgCount() != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	calleePath := site.CalleePathRef()
	if calleePath.Symbol != path.Symbol || len(calleePath.Segments) != 0 {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	// Keep the local safety checks at the consumer boundary even though the
	// binder query also fails closed. The surface must never classify a global
	// or reassigned binding as a lexical direct target.
	kind, kindOK := bindings.Kind(path.Symbol)
	if !kindOK || kind == symbol.Global || bindings.HasWrite(path.Symbol) {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	functionIdentity, exact := bindings.StableLocalFunctionIdentity(path.Symbol)
	if !exact {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	target := lexicalidentity.FunctionBody(namespace, uint64(functionIdentity))
	return target, target != (lexicalidentity.StableLexicalBodyID{})
}
